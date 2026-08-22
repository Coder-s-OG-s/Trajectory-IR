// Package tir implements portable .tir package export and import for the Go IR core.
//
// Layout and semantics match pkg/trajectory_ir/package/tir.py (README §9):
//
//	manifest.json
//	nodes.ndjson
//	seals.json
//	artifacts-manifest.json
//	COMPAT.json
//	artifacts/cas/<shard>/<hash>   // fat only
//	SIGNATURE                      // optional; trajir-pkg-sig-v1 (README §9.1)
//
// Unsigned packages remain valid by default. When SIGNATURE is present, Load
// verifies it (fail closed on tamper). Sign/Verify implement the package
// signature scheme; see signature.go.
package tir

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
)

// Package format constants match the Python reference.
const (
	PackageFormatVersion = "0.1"

	MaxZipEntries             = 10_000
	MaxUncompressedEntryBytes = 32 * 1024 * 1024  // 32 MiB
	MaxTotalUncompressedBytes = 128 * 1024 * 1024 // 128 MiB
	MaxNodes                  = 100_000
	MaxArtifacts              = 10_000
)

// Mode is the package mode: thin (no embedded bytes) or fat (CAS artifacts).
type Mode string

const (
	ModeThin Mode = "thin"
	ModeFat  Mode = "fat"
)

// COMPAT is written into every package (same keys as Python).
var COMPAT = map[string]string{
	"package_format":      PackageFormatVersion,
	"min_runtime":         "0.1.0",
	"node_id_scheme":      "sha256(tenant|trajectory|step|seq|kind|payload_hash)",
	"payload_hash_scheme": "sha256(rfc8785(payload))",
}

var requiredMembers = []string{
	"manifest.json",
	"nodes.ndjson",
	"seals.json",
	"artifacts-manifest.json",
	"COMPAT.json",
}

// Sentinel errors (compare with errors.Is / errors.As).
var (
	ErrTir          = errors.New("tir")
	ErrVerification = errors.New("tir verification")
	ErrLimit        = errors.New("tir limit")
	ErrUnsafePath   = errors.New("tir unsafe path")
)

// ArtifactRef is one artifact entry for thin or fat packages.
type ArtifactRef struct {
	LogicalPath string
	ContentHash string
	URI         *string
	Size        *int
}

// Package is an in-memory representation of a (usually verified) .tir zip.
type Package struct {
	Manifest          map[string]any
	Nodes             []map[string]any
	Seals             []map[string]any
	ArtifactsManifest []map[string]any
	ArtifactBytes     map[string][]byte // content_hash -> bytes (fat only)
	// Signature is set when the package has a valid SIGNATURE member (nil if unsigned).
	Signature *SignatureInfo
}

// ExportOptions configure Export.
type ExportOptions struct {
	Mode          Mode
	TenantID      *string // when set, only that tenant's nodes are exported
	Artifacts     []ArtifactRef
	ArtifactBytes map[string][]byte // content_hash -> bytes (required for fat)
	// SignKey, when set, must be a full ed25519 private key (64 bytes). Export then
	// writes SIGNATURE after the zip (README §9.1). A non-empty key of the wrong
	// length fails closed; it does not silently produce an unsigned package.
	SignKey    ed25519.PrivateKey
	SignerMeta SignerMeta
}

// contentHash returns SHA-256 hex of data.
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func safeZipName(name string) error {
	// Match Python _safe_zip_name: reject absolute paths, ".." segments, odd artifacts/.
	if name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	parts := strings.Split(strings.ReplaceAll(name, "\\", "/"), "/")
	if len(parts) > 0 && parts[0] == "" {
		return fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	for _, p := range parts {
		if p == ".." {
			return fmt.Errorf("%w: %q", ErrUnsafePath, name)
		}
	}
	if strings.HasPrefix(name, "artifacts/") && name != "artifacts/" && !strings.HasPrefix(name, "artifacts/cas/") {
		return fmt.Errorf("%w: unexpected artifact path %q", ErrVerification, name)
	}
	return nil
}

func marshalPretty(v any) ([]byte, error) {
	// sort_keys=True equivalent: encoding/json sorts map keys.
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func marshalLine(v any) ([]byte, error) {
	// Compact JSON with sorted map keys (encoding/json default for maps).
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	default:
		return 0, fmt.Errorf("want int, got %T", v)
	}
}

func asStepN(v any) (*int, error) {
	if v == nil {
		return nil, nil
	}
	i, err := asInt(v)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func asPayload(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{}, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("payload must be an object, got %T", v)
	}
	return m, nil
}

// verifyNodeRecord recomputes id from fields; fails on mismatch.
func verifyNodeRecord(rec map[string]any) error {
	payload, err := asPayload(rec["payload"])
	if err != nil {
		return fmt.Errorf("%w: node %v: %v", ErrVerification, rec["id"], err)
	}
	ph, err := nodes.PayloadHash(payload)
	if err != nil {
		return fmt.Errorf("%w: node %v: %v", ErrVerification, rec["id"], err)
	}
	tenant, _ := asString(rec["tenant_id"])
	traj, _ := asString(rec["trajectory_id"])
	kind, _ := asString(rec["kind"])
	stepN, err := asStepN(rec["step_n"])
	if err != nil {
		return fmt.Errorf("%w: node %v: step_n: %v", ErrVerification, rec["id"], err)
	}
	seq, err := asInt(rec["seq"])
	if err != nil {
		return fmt.Errorf("%w: node %v: seq: %v", ErrVerification, rec["id"], err)
	}
	expected, err := nodes.NodeID(tenant, traj, stepN, seq, kind, ph)
	if err != nil {
		return fmt.Errorf("%w: node %v: %v", ErrVerification, rec["id"], err)
	}
	got, _ := asString(rec["id"])
	if got != expected {
		return fmt.Errorf(
			"%w: node id mismatch kind=%s seq=%d recorded=%s expected=%s",
			ErrVerification, kind, seq, got, expected,
		)
	}
	if raw, ok := rec["payload_hash"]; ok {
		if s, _ := asString(raw); s != "" && s != ph {
			return fmt.Errorf(
				"%w: payload_hash mismatch for node %s: recorded=%s expected=%s",
				ErrVerification, got, s, ph,
			)
		}
	}
	return nil
}

func sealsFromNodes(nodeList []map[string]any) ([]map[string]any, error) {
	seals := make([]map[string]any, 0)
	for _, n := range nodeList {
		kind, _ := asString(n["kind"])
		if kind != "DECISION" {
			continue
		}
		payload, err := asPayload(n["payload"])
		if err != nil {
			return nil, err
		}
		ph, err := nodes.PayloadHash(payload)
		if err != nil {
			return nil, err
		}
		id, _ := asString(n["id"])
		seq, err := asInt(n["seq"])
		if err != nil {
			return nil, err
		}
		seals = append(seals, map[string]any{
			"type":         "DECISION_SEAL",
			"node_id":      id,
			"step_n":       n["step_n"],
			"seq":          seq,
			"content_hash": ph,
		})
	}
	return seals, nil
}

// Export writes a .tir zip for trajectoryID from nodeLog.
func Export(nodeLog *nodelog.NodeLog, trajectoryID, dest string, opts ExportOptions) (string, error) {
	if nodeLog == nil {
		return "", fmt.Errorf("%w: nil NodeLog", ErrTir)
	}
	mode := opts.Mode
	if mode == "" {
		mode = ModeThin
	}
	if mode != ModeThin && mode != ModeFat {
		return "", fmt.Errorf("%w: unsupported mode %q; use thin or fat", ErrTir, mode)
	}
	if len(opts.SignKey) != 0 && len(opts.SignKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("%w: invalid ed25519 private key size", ErrSignature)
	}

	var nodeList []map[string]any
	var err error
	if opts.TenantID != nil {
		nodeList, err = nodeLog.ListNodes(trajectoryID, *opts.TenantID)
	} else {
		nodeList, err = nodeLog.ListNodesAllTenants(trajectoryID)
	}
	if err != nil {
		return "", err
	}
	if len(nodeList) == 0 {
		return "", fmt.Errorf("%w: no nodes for trajectory_id=%q", ErrTir, trajectoryID)
	}

	exportTenant, _ := asString(nodeList[0]["tenant_id"])
	for _, n := range nodeList {
		tid, _ := asString(n["tenant_id"])
		if tid != exportTenant {
			return "", fmt.Errorf(
				"%w: trajectory contains mixed tenant_id values; pass TenantID to scope export",
				ErrTir,
			)
		}
		if err := verifyNodeRecord(n); err != nil {
			return "", err
		}
	}

	artifactBytes := map[string][]byte{}
	artifacts := opts.Artifacts
	if artifacts == nil {
		artifacts = []ArtifactRef{}
	}
	if mode == ModeFat {
		for _, ref := range artifacts {
			data, ok := opts.ArtifactBytes[ref.ContentHash]
			if !ok {
				return "", fmt.Errorf("%w: fat export missing bytes for content_hash=%s", ErrTir, ref.ContentHash)
			}
			if contentHash(data) != ref.ContentHash {
				return "", fmt.Errorf("%w: artifact bytes do not match content_hash=%s", ErrTir, ref.ContentHash)
			}
			if len(data) > MaxUncompressedEntryBytes {
				return "", fmt.Errorf("%w: artifact %s exceeds size limit", ErrLimit, ref.ContentHash)
			}
			artifactBytes[ref.ContentHash] = data
		}
		if len(artifacts) > MaxArtifacts {
			return "", fmt.Errorf("%w: too many artifacts: %d > %d", ErrLimit, len(artifacts), MaxArtifacts)
		}
	}

	seals, err := sealsFromNodes(nodeList)
	if err != nil {
		return "", err
	}
	manifest := map[string]any{
		"package_format": PackageFormatVersion,
		"trajectory_id":  trajectoryID,
		"tenant_id":      exportTenant,
		"mode":           string(mode),
		"node_count":     len(nodeList),
		"seal_count":     len(seals),
		"signature":      nil,
		"redacted":       false,
	}

	artManifest := make([]map[string]any, 0, len(artifacts))
	for _, a := range artifacts {
		entry := map[string]any{
			"logical_path": a.LogicalPath,
			"content_hash": a.ContentHash,
			"uri":          nil,
			"size":         nil,
		}
		if a.URI != nil {
			entry["uri"] = *a.URI
		}
		if a.Size != nil {
			entry["size"] = *a.Size
		} else if data, ok := artifactBytes[a.ContentHash]; ok {
			entry["size"] = len(data)
		}
		artManifest = append(artManifest, entry)
	}

	destPath := dest
	if filepath.Ext(destPath) != ".tir" {
		destPath = destPath + ".tir"
	}

	f, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	fileClosed := false
	defer func() {
		if !fileClosed {
			_ = f.Close()
		}
	}()

	zw := zip.NewWriter(f)
	writeFile := func(name string, body []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(body)
		return err
	}

	mb, err := marshalPretty(manifest)
	if err != nil {
		_ = zw.Close()
		return "", err
	}
	if err := writeFile("manifest.json", mb); err != nil {
		_ = zw.Close()
		return "", err
	}
	cb, err := marshalPretty(COMPAT)
	if err != nil {
		_ = zw.Close()
		return "", err
	}
	if err := writeFile("COMPAT.json", cb); err != nil {
		_ = zw.Close()
		return "", err
	}
	sb, err := marshalPretty(seals)
	if err != nil {
		_ = zw.Close()
		return "", err
	}
	if err := writeFile("seals.json", sb); err != nil {
		_ = zw.Close()
		return "", err
	}
	ab, err := marshalPretty(artManifest)
	if err != nil {
		_ = zw.Close()
		return "", err
	}
	if err := writeFile("artifacts-manifest.json", ab); err != nil {
		_ = zw.Close()
		return "", err
	}

	var ndjson bytes.Buffer
	for _, n := range nodeList {
		line, err := marshalLine(n)
		if err != nil {
			_ = zw.Close()
			return "", err
		}
		ndjson.Write(line)
	}
	if err := writeFile("nodes.ndjson", ndjson.Bytes()); err != nil {
		_ = zw.Close()
		return "", err
	}

	if mode == ModeFat {
		for h, data := range artifactBytes {
			shard := h
			if len(shard) >= 2 {
				shard = h[:2]
			}
			name := fmt.Sprintf("artifacts/cas/%s/%s", shard, h)
			if err := writeFile(name, data); err != nil {
				_ = zw.Close()
				return "", err
			}
		}
	}

	if err := zw.Close(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		fileClosed = true
		return "", err
	}
	fileClosed = true

	if len(opts.SignKey) != 0 {
		if err := Sign(destPath, opts.SignKey, opts.SignerMeta); err != nil {
			return "", err
		}
	}
	return destPath, nil
}

// Load reads and verifies a .tir zip without writing to a NodeLog.
// Verification is always on; use LoadUnverified for explicit opt-out.
func Load(path string) (*Package, error) {
	return loadImpl(path, true)
}

// LoadUnverified loads without hash verification. UNSAFE for untrusted input.
func LoadUnverified(path string) (*Package, error) {
	return loadImpl(path, false)
}

func loadImpl(path string, verify bool) (*Package, error) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return nil, fmt.Errorf("%w: package not found: %s", ErrTir, path)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open zip: %v", ErrTir, err)
	}
	defer zr.Close()

	if len(zr.File) > MaxZipEntries {
		return nil, fmt.Errorf("%w: too many zip entries: %d > %d", ErrLimit, len(zr.File), MaxZipEntries)
	}

	// Reject duplicate member names up front (CWE-436). collectZipMembers also
	// checks this, but unsigned Load skips that path to avoid a second decompress.
	byName := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		name := f.Name
		if strings.HasSuffix(name, "/") {
			continue
		}
		if err := safeZipName(name); err != nil {
			return nil, err
		}
		if _, exists := byName[name]; exists {
			return nil, fmt.Errorf("%w: duplicate zip member %q", ErrTir, name)
		}
		byName[name] = f
	}
	for _, req := range requiredMembers {
		if _, ok := byName[req]; !ok {
			return nil, fmt.Errorf("%w: package missing files: %s", ErrTir, req)
		}
	}

	budget := MaxTotalUncompressedBytes
	readMember := func(name string) ([]byte, error) {
		f, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("%w: missing %s", ErrTir, name)
		}
		if f.UncompressedSize64 > MaxUncompressedEntryBytes {
			return nil, fmt.Errorf(
				"%w: zip member %q uncompressed size %d exceeds limit %d",
				ErrLimit, name, f.UncompressedSize64, MaxUncompressedEntryBytes,
			)
		}
		if int(f.UncompressedSize64) > budget {
			return nil, fmt.Errorf(
				"%w: zip total uncompressed size would exceed limit %d",
				ErrLimit, MaxTotalUncompressedBytes,
			)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		// Cap read so a lying local header cannot OOM us.
		limited := io.LimitReader(rc, int64(MaxUncompressedEntryBytes)+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			return nil, err
		}
		if len(data) > MaxUncompressedEntryBytes {
			return nil, fmt.Errorf("%w: zip member %q expanded beyond per-entry limit", ErrLimit, name)
		}
		budget -= len(data)
		return data, nil
	}

	// Also allow reading artifact members not in the initial required set.
	readAny := func(f *zip.File) ([]byte, error) {
		if f.UncompressedSize64 > MaxUncompressedEntryBytes {
			return nil, fmt.Errorf(
				"%w: zip member %q uncompressed size %d exceeds limit %d",
				ErrLimit, f.Name, f.UncompressedSize64, MaxUncompressedEntryBytes,
			)
		}
		if int(f.UncompressedSize64) > budget {
			return nil, fmt.Errorf(
				"%w: zip total uncompressed size would exceed limit %d",
				ErrLimit, MaxTotalUncompressedBytes,
			)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		limited := io.LimitReader(rc, int64(MaxUncompressedEntryBytes)+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			return nil, err
		}
		if len(data) > MaxUncompressedEntryBytes {
			return nil, fmt.Errorf("%w: zip member %q expanded beyond per-entry limit", ErrLimit, f.Name)
		}
		budget -= len(data)
		return data, nil
	}

	rawManifest, err := readMember("manifest.json")
	if err != nil {
		return nil, err
	}
	var manifest map[string]any
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		return nil, fmt.Errorf("%w: manifest.json: %v", ErrTir, err)
	}

	rawSeals, err := readMember("seals.json")
	if err != nil {
		return nil, err
	}
	var seals []map[string]any
	if err := json.Unmarshal(rawSeals, &seals); err != nil {
		return nil, fmt.Errorf("%w: seals.json: %v", ErrTir, err)
	}

	rawArtMan, err := readMember("artifacts-manifest.json")
	if err != nil {
		return nil, err
	}
	var artManifest []map[string]any
	if err := json.Unmarshal(rawArtMan, &artManifest); err != nil {
		return nil, fmt.Errorf("%w: artifacts-manifest.json: %v", ErrTir, err)
	}
	if len(artManifest) > MaxArtifacts {
		return nil, fmt.Errorf("%w: too many artifact manifest entries: > %d", ErrLimit, MaxArtifacts)
	}

	if _, err := readMember("COMPAT.json"); err != nil {
		return nil, err
	}

	rawNodes, err := readMember("nodes.ndjson")
	if err != nil {
		return nil, err
	}
	nodeList := make([]map[string]any, 0)
	for _, line := range strings.Split(string(rawNodes), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("%w: nodes.ndjson: %v", ErrTir, err)
		}
		nodeList = append(nodeList, rec)
		if len(nodeList) > MaxNodes {
			return nil, fmt.Errorf("%w: too many nodes: > %d", ErrLimit, MaxNodes)
		}
	}

	artifactBytes := map[string][]byte{}
	for _, f := range zr.File {
		name := f.Name
		if strings.HasPrefix(name, "artifacts/cas/") && !strings.HasSuffix(name, "/") {
			data, err := readAny(f)
			if err != nil {
				return nil, err
			}
			h := filepath.Base(name)
			if len(h) != 64 || !isHex(h) {
				return nil, fmt.Errorf("%w: invalid artifact content hash name: %q", ErrVerification, h)
			}
			if contentHash(data) != h {
				return nil, fmt.Errorf(
					"%w: embedded artifact name %s does not match content hash",
					ErrVerification, h,
				)
			}
			artifactBytes[h] = data
			if len(artifactBytes) > MaxArtifacts {
				return nil, fmt.Errorf("%w: too many embedded artifacts: > %d", ErrLimit, MaxArtifacts)
			}
		}
	}

	if verify {
		if len(nodeList) == 0 {
			return nil, fmt.Errorf("%w: package has no nodes", ErrVerification)
		}
		for _, n := range nodeList {
			if err := verifyNodeRecord(n); err != nil {
				return nil, err
			}
		}
		expectedSeals, err := sealsFromNodes(nodeList)
		if err != nil {
			return nil, err
		}
		byID := make(map[string]map[string]any, len(seals))
		for _, s := range seals {
			if id, ok := asString(s["node_id"]); ok {
				byID[id] = s
			}
		}
		for _, exp := range expectedSeals {
			id, _ := asString(exp["node_id"])
			got, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("%w: missing seal for decision node %s", ErrVerification, id)
			}
			wantHash, _ := asString(exp["content_hash"])
			gotHash, _ := asString(got["content_hash"])
			if gotHash != wantHash {
				return nil, fmt.Errorf("%w: seal content_hash mismatch for %s", ErrVerification, id)
			}
		}
		mode, _ := asString(manifest["mode"])
		if mode == "" {
			mode = string(ModeThin)
		}
		if mode == string(ModeFat) {
			for _, entry := range artManifest {
				h, _ := asString(entry["content_hash"])
				if _, ok := artifactBytes[h]; !ok {
					return nil, fmt.Errorf("%w: fat package missing artifact bytes %s", ErrVerification, h)
				}
			}
		}
		if mode == string(ModeThin) && len(artifactBytes) > 0 {
			return nil, fmt.Errorf("%w: thin package must not embed artifact bytes", ErrVerification)
		}
	}

	// Signature check after hash/seal integrity (README §9.1 order).
	// Use the already-open archive only — never re-open path (TOCTOU / CWE-367).
	// Present-but-invalid SIGNATURE fails even for LoadUnverified (tamper).
	// Unsigned packages skip the second decompress pass (no SIGNATURE member).
	var sigInfo *SignatureInfo
	if _, hasSig := byName[SignatureMemberName]; hasSig {
		sigInfo, err = verifySignatureFromZipFiles(zr.File, VerifyOptions{})
		if err != nil {
			return nil, err
		}
	}

	return &Package{
		Manifest:          manifest,
		Nodes:             nodeList,
		Seals:             seals,
		ArtifactsManifest: artManifest,
		ArtifactBytes:     artifactBytes,
		Signature:         sigInfo,
	}, nil
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// Import verifies a package and appends its nodes into nodeLog (idempotent by id).
// Verification cannot be disabled — forged packages must not enter a durable log.
func Import(path string, nodeLog *nodelog.NodeLog) (*Package, error) {
	if nodeLog == nil {
		return nil, fmt.Errorf("%w: nil NodeLog", ErrTir)
	}
	pkg, err := Load(path)
	if err != nil {
		return nil, err
	}
	for _, n := range pkg.Nodes {
		kind, _ := asString(n["kind"])
		traj, _ := asString(n["trajectory_id"])
		tenant, _ := asString(n["tenant_id"])
		stepN, err := asStepN(n["step_n"])
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrVerification, err)
		}
		seq, err := asInt(n["seq"])
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrVerification, err)
		}
		payload, err := asPayload(n["payload"])
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrVerification, err)
		}
		node, err := nodes.NewNode(kind, traj, tenant, stepN, seq, payload)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrVerification, err)
		}
		wantID, _ := asString(n["id"])
		if node.ID != wantID {
			return nil, fmt.Errorf(
				"%w: rebuilt node id %s != package id %s",
				ErrVerification, node.ID, wantID,
			)
		}
		if _, err := nodeLog.Append(kind, stepN, payload, traj, tenant, seq); err != nil {
			return nil, err
		}
	}
	return pkg, nil
}
