// Package signature support for .tir (README §9.1, trajir-pkg-sig-v1).
//
// File-only SIGNATURE zip member; manifest.signature stays null.
// Payload: trajir-pkg-payload-v1 over all members except SIGNATURE.
// Crypto: domain-separated Ed25519.
package tir

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Signature scheme constants (README §9.1).
const (
	SignatureMemberName = "SIGNATURE"
	SchemeV1            = "trajir-pkg-sig-v1"
	PayloadAlgV1        = "trajir-pkg-payload-v1"
	AlgEd25519          = "ed25519"
)

// ErrSignature indicates package signature failure (missing when required, tamper, unknown scheme).
var ErrSignature = errors.New("tir signature")

// SignerMeta is optional metadata written into the SIGNATURE document.
type SignerMeta struct {
	// ID is a free-form signer identity (optional).
	ID string
	// SignedAt overrides the timestamp; zero means time.Now().UTC().
	SignedAt time.Time
}

// SignatureDocument is the JSON body of the SIGNATURE zip member.
type SignatureDocument struct {
	Scheme      string         `json:"scheme"`
	PayloadAlg  string         `json:"payload_alg"`
	PayloadHash string         `json:"payload_hash"`
	Alg         string         `json:"alg"`
	PublicKey   string         `json:"public_key"`
	Signature   string         `json:"signature"`
	SignedAt    string         `json:"signed_at"`
	Signer      SignerIdentity `json:"signer"`
	Extensions  map[string]any `json:"extensions"`
}

// SignerIdentity is the optional signer block inside SignatureDocument.
type SignerIdentity struct {
	ID    string `json:"id,omitempty"`
	KeyID string `json:"key_id,omitempty"`
}

// SignatureInfo is returned by Verify and attached to Package when Load finds a valid SIGNATURE.
type SignatureInfo struct {
	Document    *SignatureDocument
	PayloadHash [32]byte
	PublicKey   ed25519.PublicKey
	KeyID       string
	SignerID    string
}

// VerifyOptions control Verify (and optional strict policy for callers).
type VerifyOptions struct {
	// RequireSignature fails when the package has no SIGNATURE member.
	RequireSignature bool
	// TrustedKeys, when non-empty, requires the package public key to match one entry.
	TrustedKeys []ed25519.PublicKey
	// TrustedKeyIDs, when non-empty, requires document signer.key_id to be listed
	// (in addition to TrustedKeys if both are set — key must satisfy both).
	TrustedKeyIDs []string
}

// KeyID returns the recommended key fingerprint: first 16 hex chars of SHA-256(public_key raw).
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])[:16]
}

// DomainSeparatedMessage returns SHA-256("trajir-pkg-sig-v1" || 0x00 || payload_hash_raw).
func DomainSeparatedMessage(payloadHash [32]byte) [32]byte {
	h := sha256.New()
	_, _ = h.Write([]byte(SchemeV1))
	_, _ = h.Write([]byte{0x00})
	_, _ = h.Write(payloadHash[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// PayloadHashFromMembers computes trajir-pkg-payload-v1 over members (must not include SIGNATURE).
func PayloadHashFromMembers(members map[string][]byte) ([32]byte, error) {
	var zero [32]byte
	if members == nil {
		return zero, fmt.Errorf("%w: nil members", ErrSignature)
	}
	names := make([]string, 0, len(members))
	for name := range members {
		if name == SignatureMemberName {
			continue
		}
		if err := safeZipName(name); err != nil {
			return zero, err
		}
		names = append(names, name)
	}
	sort.Strings(names)

	stream := sha256.New()
	var lenBuf [8]byte
	for _, name := range names {
		body := members[name]
		nameBytes := []byte(name)
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(nameBytes)))
		if _, err := stream.Write(lenBuf[:]); err != nil {
			return zero, err
		}
		if _, err := stream.Write(nameBytes); err != nil {
			return zero, err
		}
		sum := sha256.Sum256(body)
		if _, err := stream.Write(sum[:]); err != nil {
			return zero, err
		}
	}
	var out [32]byte
	copy(out[:], stream.Sum(nil))
	return out, nil
}

// Sign writes or replaces the SIGNATURE member of an existing .tir package.
// manifest.json is left unchanged (signature field stays null).
func Sign(path string, privateKey ed25519.PrivateKey, meta SignerMeta) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("%w: invalid ed25519 private key size", ErrSignature)
	}
	members, err := readZipMembersExceptSignature(path)
	if err != nil {
		return err
	}
	payloadHash, err := PayloadHashFromMembers(members)
	if err != nil {
		return err
	}
	pub := privateKey.Public().(ed25519.PublicKey)
	msg := DomainSeparatedMessage(payloadHash)
	sig := ed25519.Sign(privateKey, msg[:])

	signedAt := meta.SignedAt
	if signedAt.IsZero() {
		signedAt = time.Now().UTC()
	}
	doc := SignatureDocument{
		Scheme:      SchemeV1,
		PayloadAlg:  PayloadAlgV1,
		PayloadHash: hex.EncodeToString(payloadHash[:]),
		Alg:         AlgEd25519,
		PublicKey:   base64.StdEncoding.EncodeToString(pub),
		Signature:   base64.StdEncoding.EncodeToString(sig),
		SignedAt:    signedAt.UTC().Format(time.RFC3339),
		Signer: SignerIdentity{
			ID:    meta.ID,
			KeyID: KeyID(pub),
		},
		Extensions: map[string]any{},
	}
	docBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: marshal SIGNATURE: %v", ErrSignature, err)
	}
	docBytes = append(docBytes, '\n')
	return rewriteZipWithSignature(path, members, docBytes)
}

// Verify checks package signature policy for path.
// When SIGNATURE is absent and RequireSignature is false, returns (nil, nil).
//
// Prefer verifySignatureFromZipFiles when the archive is already open (Load/Import)
// so signature metadata cannot come from a different file than the parsed members
// (TOCTOU / CWE-367).
func Verify(path string, opts VerifyOptions) (*SignatureInfo, error) {
	members, sigRaw, err := readZipMembersAndSignature(path)
	if err != nil {
		return nil, err
	}
	return verifyFromMembers(members, sigRaw, opts)
}

// verifySignatureFromZipFiles verifies SIGNATURE against members from an already-open
// zip.Reader (same archive instance Load is parsing). Do not re-open path here.
func verifySignatureFromZipFiles(files []*zip.File, opts VerifyOptions) (*SignatureInfo, error) {
	members, sigRaw, err := collectZipMembers(files)
	if err != nil {
		return nil, err
	}
	return verifyFromMembers(members, sigRaw, opts)
}

func verifyFromMembers(members map[string][]byte, sigRaw []byte, opts VerifyOptions) (*SignatureInfo, error) {
	if sigRaw == nil {
		if opts.RequireSignature {
			return nil, fmt.Errorf("%w: package is unsigned (SIGNATURE missing)", ErrSignature)
		}
		return nil, nil
	}
	return verifySignatureBytes(members, sigRaw, opts)
}

func verifySignatureBytes(members map[string][]byte, sigRaw []byte, opts VerifyOptions) (*SignatureInfo, error) {
	var doc SignatureDocument
	if err := json.Unmarshal(sigRaw, &doc); err != nil {
		return nil, fmt.Errorf("%w: SIGNATURE JSON: %v", ErrSignature, err)
	}
	if doc.Scheme != SchemeV1 {
		return nil, fmt.Errorf("%w: unknown scheme %q", ErrSignature, doc.Scheme)
	}
	if doc.PayloadAlg != PayloadAlgV1 {
		return nil, fmt.Errorf("%w: unknown payload_alg %q", ErrSignature, doc.PayloadAlg)
	}
	if doc.Alg != AlgEd25519 {
		return nil, fmt.Errorf("%w: unknown alg %q", ErrSignature, doc.Alg)
	}

	wantHash, err := PayloadHashFromMembers(members)
	if err != nil {
		return nil, err
	}
	gotHashHex := strings.ToLower(strings.TrimSpace(doc.PayloadHash))
	wantHashHex := hex.EncodeToString(wantHash[:])
	if gotHashHex != wantHashHex {
		return nil, fmt.Errorf("%w: payload_hash mismatch", ErrSignature)
	}

	pubRaw, err := base64.StdEncoding.DecodeString(doc.PublicKey)
	if err != nil || len(pubRaw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: invalid public_key", ErrSignature)
	}
	pub := ed25519.PublicKey(pubRaw)

	sig, err := base64.StdEncoding.DecodeString(doc.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: invalid signature encoding", ErrSignature)
	}

	msg := DomainSeparatedMessage(wantHash)
	if !ed25519.Verify(pub, msg[:], sig) {
		return nil, fmt.Errorf("%w: ed25519 verification failed", ErrSignature)
	}

	keyID := doc.Signer.KeyID
	if keyID == "" {
		keyID = KeyID(pub)
	} else if keyID != KeyID(pub) {
		// Allow document key_id if it matches recomputed; reject mismatch as tamper.
		return nil, fmt.Errorf("%w: signer.key_id does not match public_key", ErrSignature)
	}

	if len(opts.TrustedKeys) > 0 {
		ok := false
		for _, tk := range opts.TrustedKeys {
			if bytes.Equal(tk, pub) {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("%w: public key not in trust store", ErrSignature)
		}
	}
	if len(opts.TrustedKeyIDs) > 0 {
		ok := false
		for _, id := range opts.TrustedKeyIDs {
			if id == keyID {
				ok = true
				break
			}
		}
		if !ok {
			return nil, fmt.Errorf("%w: key_id %q not in trust store", ErrSignature, keyID)
		}
	}

	return &SignatureInfo{
		Document:    &doc,
		PayloadHash: wantHash,
		PublicKey:   pub,
		KeyID:       keyID,
		SignerID:    doc.Signer.ID,
	}, nil
}

func readZipMembersExceptSignature(path string) (map[string][]byte, error) {
	members, _, err := readZipMembersAndSignature(path)
	return members, err
}

func readZipMembersAndSignature(path string) (members map[string][]byte, signature []byte, err error) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return nil, nil, fmt.Errorf("%w: package not found: %s", ErrTir, path)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: open zip: %v", ErrTir, err)
	}
	defer zr.Close()

	if len(zr.File) > MaxZipEntries {
		return nil, nil, fmt.Errorf("%w: too many zip entries: %d > %d", ErrLimit, len(zr.File), MaxZipEntries)
	}
	return collectZipMembers(zr.File)
}

// collectZipMembers reads all non-directory entries from an open archive.
// Rejects duplicate member names, including duplicate SIGNATURE (CWE-436).
func collectZipMembers(files []*zip.File) (members map[string][]byte, signature []byte, err error) {
	if len(files) > MaxZipEntries {
		return nil, nil, fmt.Errorf("%w: too many zip entries: %d > %d", ErrLimit, len(files), MaxZipEntries)
	}
	members = make(map[string][]byte)
	budget := MaxTotalUncompressedBytes
	for _, f := range files {
		name := f.Name
		if strings.HasSuffix(name, "/") {
			continue
		}
		if err := safeZipName(name); err != nil {
			return nil, nil, err
		}
		if f.UncompressedSize64 > MaxUncompressedEntryBytes {
			return nil, nil, fmt.Errorf(
				"%w: zip member %q uncompressed size %d exceeds limit %d",
				ErrLimit, name, f.UncompressedSize64, MaxUncompressedEntryBytes,
			)
		}
		if int(f.UncompressedSize64) > budget {
			return nil, nil, fmt.Errorf(
				"%w: zip total uncompressed size would exceed limit %d",
				ErrLimit, MaxTotalUncompressedBytes,
			)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		limited := io.LimitReader(rc, int64(MaxUncompressedEntryBytes)+1)
		data, err := io.ReadAll(limited)
		_ = rc.Close()
		if err != nil {
			return nil, nil, err
		}
		if len(data) > MaxUncompressedEntryBytes {
			return nil, nil, fmt.Errorf("%w: zip member %q expanded beyond per-entry limit", ErrLimit, name)
		}
		budget -= len(data)
		if name == SignatureMemberName {
			if signature != nil {
				return nil, nil, fmt.Errorf("%w: duplicate zip member %q", ErrTir, name)
			}
			signature = data
			continue
		}
		if _, exists := members[name]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate zip member %q", ErrTir, name)
		}
		members[name] = data
	}
	return members, signature, nil
}

func rewriteZipWithSignature(path string, members map[string][]byte, signatureJSON []byte) error {
	dir := filepath.Dir(path)
	// Preserve destination mode across CreateTemp (which uses 0600) so signed
	// exports keep the same permissions as the unsigned zip Export wrote.
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%w: stat package for mode: %v", ErrSignature, err)
	}
	origMode := st.Mode().Perm()
	tmp, err := os.CreateTemp(dir, "tir-sign-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		_ = tmp.Close()
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	zw := zip.NewWriter(tmp)
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write(members[name]); err != nil {
			return err
		}
	}
	w, err := zw.Create(SignatureMemberName)
	if err != nil {
		return err
	}
	if _, err := w.Write(signatureJSON); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	// Match go/trajir/cas Put: durable bytes before rename so a crash after
	// replaceFile cannot leave a truncated package that reported success.
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, origMode); err != nil {
		return fmt.Errorf("%w: preserve package mode: %v", ErrSignature, err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		return err
	}
	success = true
	return nil
}

// afterBackupHook is an optional test seam run after the original package is
// moved aside and before the temp is moved into place. Production leaves it nil.
var afterBackupHook func(dest string)

// replaceFile installs src at dest without deleting dest until src is in place.
// On platforms where rename cannot overwrite (Windows), dest is moved to a
// sibling backup first; if the final rename fails, the backup is restored.
func replaceFile(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	return replaceFileViaBackup(src, dest)
}

func replaceFileViaBackup(src, dest string) error {
	bak := dest + ".trajir-replace-bak"
	_ = os.Remove(bak)
	if err := os.Rename(dest, bak); err != nil {
		return fmt.Errorf("%w: replace package (move aside): %v", ErrSignature, err)
	}
	if afterBackupHook != nil {
		afterBackupHook(dest)
	}
	if err := os.Rename(src, dest); err != nil {
		if rbErr := os.Rename(bak, dest); rbErr != nil {
			return fmt.Errorf("%w: replace package: %v (restore failed: %v)", ErrSignature, err, rbErr)
		}
		return fmt.Errorf("%w: replace package: %v", ErrSignature, err)
	}
	_ = os.Remove(bak)
	return nil
}
