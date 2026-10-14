package tir_test

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

// testOnlySeed derives a deterministic Ed25519 key for tests and goldens.
// This is NOT a production key; private material is never committed as a file.
func testOnlySeed() ed25519.PrivateKey {
	sum := sha256.Sum256([]byte("trajir-pkg-sig-v1-test-vector-seed"))
	return ed25519.NewKeyFromSeed(sum[:])
}

func TestPayloadHashAndDomainMessageGolden(t *testing.T) {
	// Fixed member set for cross-language golden (Phase C).
	members := map[string][]byte{
		"COMPAT.json":             []byte("{\n  \"package_format\": \"0.1\"\n}\n"),
		"manifest.json":           []byte("{\n  \"mode\": \"thin\",\n  \"signature\": null\n}\n"),
		"nodes.ndjson":            []byte("{\"id\":\"abc\"}\n"),
		"seals.json":              []byte("[]\n"),
		"artifacts-manifest.json": []byte("[]\n"),
	}

	payloadHash, err := tir.PayloadHashFromMembers(members)
	if err != nil {
		t.Fatal(err)
	}
	domain := tir.DomainSeparatedMessage(payloadHash)

	// Locked against testdata/sig_v1/payload_golden.json for Python Phase C parity.
	gotPayload := hex.EncodeToString(payloadHash[:])
	gotDomain := hex.EncodeToString(domain[:])

	goldenPath := filepath.Join("testdata", "sig_v1", "payload_golden.json")
	type golden struct {
		MembersB64       map[string]string `json:"members_b64"`
		PayloadHashHex   string            `json:"payload_hash_hex"`
		DomainMessageHex string            `json:"domain_message_hex"`
		SeedNote         string            `json:"seed_note"`
		PublicKeyB64     string            `json:"public_key_b64"`
		SignatureB64     string            `json:"signature_b64"`
		KeyID            string            `json:"key_id"`
	}

	priv := testOnlySeed()
	pub := priv.Public().(ed25519.PublicKey)
	sig := ed25519.Sign(priv, domain[:])

	existing, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (regenerate by computing members and committing)", goldenPath, err)
	}
	var old golden
	if err := json.Unmarshal(existing, &old); err != nil {
		t.Fatalf("golden parse: %v", err)
	}
	if old.PayloadHashHex != gotPayload || old.DomainMessageHex != gotDomain {
		t.Fatalf("payload golden mismatch:\n  got payload %s\n want payload %s\n  got domain  %s\n want domain  %s\n(update testdata if scheme change is intentional)",
			gotPayload, old.PayloadHashHex, gotDomain, old.DomainMessageHex)
	}
	wantPub := base64.StdEncoding.EncodeToString(pub)
	wantSig := base64.StdEncoding.EncodeToString(sig)
	if old.PublicKeyB64 != wantPub || old.SignatureB64 != wantSig {
		t.Fatalf("signature golden mismatch (public_key/signature); update testdata if intentional")
	}
	if old.KeyID != tir.KeyID(pub) {
		t.Fatalf("key_id golden mismatch: got %s want %s", old.KeyID, tir.KeyID(pub))
	}
	if !ed25519.Verify(pub, domain[:], sig) {
		t.Fatal("test vector signature does not verify")
	}
	_ = members
}

func TestSignVerifyThinPackage(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)

	out := filepath.Join(t.TempDir(), "signed.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{
		Mode:       tir.ModeThin,
		SignKey:    priv,
		SignerMeta: tir.SignerMeta{ID: "test-signer", SignedAt: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := tir.Verify(path, tir.VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if info == nil {
		t.Fatal("expected signature info")
	}
	if info.SignerID != "test-signer" {
		t.Fatalf("signer id=%q", info.SignerID)
	}
	if info.Document.Scheme != tir.SchemeV1 {
		t.Fatalf("scheme=%q", info.Document.Scheme)
	}
	if info.KeyID != tir.KeyID(priv.Public().(ed25519.PublicKey)) {
		t.Fatalf("key_id=%q", info.KeyID)
	}

	pkg, err := tir.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Signature == nil {
		t.Fatal("Load should attach Signature")
	}
	// manifest.signature stays null
	if pkg.Manifest["signature"] != nil {
		t.Fatalf("manifest.signature should be null, got %v", pkg.Manifest["signature"])
	}
}

func TestUnsignedStillLoads(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "unsigned.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}
	info, err := tir.Verify(path, tir.VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if info != nil {
		t.Fatal("unsigned should return nil SignatureInfo")
	}
	if _, err := tir.Verify(path, tir.VerifyOptions{RequireSignature: true}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("require signature: %v", err)
	}
	pkg, err := tir.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Signature != nil {
		t.Fatal("unsigned package should have nil Signature")
	}
}

func TestTamperInvalidatesSignature(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "signed.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin, SignKey: priv})
	if err != nil {
		t.Fatal(err)
	}

	// Mutate COMPAT.json while keeping valid JSON so Load reaches signature check.
	if err := mutateZipMember(path, "COMPAT.json", func(b []byte) []byte {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		m["min_runtime"] = "9.9.9"
		out, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return append(out, '\n')
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := tir.Verify(path, tir.VerifyOptions{}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected signature error, got %v", err)
	}
	if _, err := tir.Load(path); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("Load should fail on bad signature: %v", err)
	}
}

func TestTrustStoreRejectsUnknownKey(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "signed.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin, SignKey: priv})
	if err != nil {
		t.Fatal(err)
	}

	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tir.Verify(path, tir.VerifyOptions{TrustedKeys: []ed25519.PublicKey{otherPub}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected trust failure: %v", err)
	}
	// Trusted correct key succeeds.
	if _, err := tir.Verify(path, tir.VerifyOptions{
		TrustedKeys: []ed25519.PublicKey{priv.Public().(ed25519.PublicKey)},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadWithOptions_RejectsUntrustedSigner(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "signed.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin, SignKey: priv})
	if err != nil {
		t.Fatal(err)
	}

	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	opts := tir.VerifyOptions{
		TrustedKeys: []ed25519.PublicKey{otherPub},
	}
	if _, err := tir.LoadWithOptions(path, opts); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for untrusted signer, got %v", err)
	}
}

func TestLoadWithOptions_EnforcesRequireSignature(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "unsigned.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}

	opts := tir.VerifyOptions{
		RequireSignature: true,
	}
	if _, err := tir.LoadWithOptions(path, opts); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for unsigned package, got %v", err)
	}
}

func TestImportWithOptions_RejectsUntrustedSigner(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "signed.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin, SignKey: priv})
	if err != nil {
		t.Fatal(err)
	}

	otherPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	dest := openLog(t, "dest.sqlite")
	opts := tir.ImportOptions{
		Verify: tir.VerifyOptions{
			TrustedKeys: []ed25519.PublicKey{otherPub},
		},
	}
	if _, err := tir.ImportWithOptions(path, dest, opts); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for untrusted signer, got %v", err)
	}
	nodes, err := dest.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes in dest log on failed import, got %d", len(nodes))
	}
}

func TestImportWithOptions_AcceptsTrustedSigner(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "signed.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin, SignKey: priv})
	if err != nil {
		t.Fatal(err)
	}

	dest := openLog(t, "dest.sqlite")
	opts := tir.ImportOptions{
		Verify: tir.VerifyOptions{
			TrustedKeys: []ed25519.PublicKey{priv.Public().(ed25519.PublicKey)},
		},
	}
	pkg, err := tir.ImportWithOptions(path, dest, opts)
	if err != nil {
		t.Fatalf("expected successful import, got %v", err)
	}
	if pkg.Signature == nil {
		t.Fatal("expected package signature info, got nil")
	}
}

func TestImportWithOptions_EnforcesRequireSignature(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "unsigned.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}

	dest := openLog(t, "dest.sqlite")
	opts := tir.ImportOptions{
		Verify: tir.VerifyOptions{
			RequireSignature: true,
		},
	}
	if _, err := tir.ImportWithOptions(path, dest, opts); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for unsigned package, got %v", err)
	}
	nodes, err := dest.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes in dest log on failed import, got %d", len(nodes))
	}
}

func TestImportWithOptions_StrippedSignatureFailsTrustStore(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "signed.tir")
	priv := testOnlySeed()
	pub := priv.Public().(ed25519.PublicKey)
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{
		Mode:    tir.ModeThin,
		SignKey: priv,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := stripZipMember(path, tir.SignatureMemberName); err != nil {
		t.Fatal(err)
	}

	dest := openLog(t, "dest.sqlite")
	opts := tir.ImportOptions{
		Verify: tir.VerifyOptions{
			TrustedKeys:      []ed25519.PublicKey{pub},
			RequireSignature: false,
		},
	}
	if _, err := tir.ImportWithOptions(path, dest, opts); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for stripped signature with trust store, got %v", err)
	}
	nodes, err := dest.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected import not to write nodes, found %d", len(nodes))
	}

	// Also verify with TrustedKeyIDs while RequireSignature is false
	destKeyID := openLog(t, "dest-keyid.sqlite")
	optsKeyID := tir.ImportOptions{
		Verify: tir.VerifyOptions{
			TrustedKeyIDs:    []string{tir.KeyID(pub)},
			RequireSignature: false,
		},
	}
	if _, err := tir.ImportWithOptions(path, destKeyID, optsKeyID); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for stripped signature with TrustedKeyIDs, got %v", err)
	}
	nodesKeyID, err := destKeyID.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodesKeyID) != 0 {
		t.Fatalf("expected import not to write nodes with TrustedKeyIDs, found %d", len(nodesKeyID))
	}
}

func TestLoadWithOptions_StrippedSignatureFailsTrustStore(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "signed.tir")
	priv := testOnlySeed()
	pub := priv.Public().(ed25519.PublicKey)
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{
		Mode:    tir.ModeThin,
		SignKey: priv,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := stripZipMember(path, tir.SignatureMemberName); err != nil {
		t.Fatal(err)
	}

	opts := tir.VerifyOptions{
		TrustedKeys:      []ed25519.PublicKey{pub},
		RequireSignature: false,
	}
	if _, err := tir.LoadWithOptions(path, opts); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for stripped signature from LoadWithOptions, got %v", err)
	}

	optsKeyID := tir.VerifyOptions{
		TrustedKeyIDs:    []string{tir.KeyID(pub)},
		RequireSignature: false,
	}
	if _, err := tir.LoadWithOptions(path, optsKeyID); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for stripped signature with TrustedKeyIDs from LoadWithOptions, got %v", err)
	}

	// Verify also fails
	if _, err := tir.Verify(path, opts); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from Verify, got %v", err)
	}
	if _, err := tir.Verify(path, optsKeyID); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from Verify with TrustedKeyIDs, got %v", err)
	}
}

func TestSignStandaloneAfterExport(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "later.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}
	priv := testOnlySeed()
	if err := tir.Sign(path, priv, tir.SignerMeta{ID: "late"}); err != nil {
		t.Fatal(err)
	}
	info, err := tir.Verify(path, tir.VerifyOptions{RequireSignature: true})
	if err != nil {
		t.Fatal(err)
	}
	if info.SignerID != "late" {
		t.Fatalf("signer=%q", info.SignerID)
	}
}

func TestSignFatPackage(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	data := []byte("artifact-bytes")
	h := sha256.Sum256(data)
	hashHex := hex.EncodeToString(h[:])
	out := filepath.Join(t.TempDir(), "fat.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{
		Mode:    tir.ModeFat,
		SignKey: priv,
		Artifacts: []tir.ArtifactRef{{
			LogicalPath: "out.txt",
			ContentHash: hashHex,
		}},
		ArtifactBytes: map[string][]byte{hashHex: data},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tir.Verify(path, tir.VerifyOptions{RequireSignature: true}); err != nil {
		t.Fatal(err)
	}
	pkg, err := tir.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Signature == nil {
		t.Fatal("expected signature on fat package")
	}
}

func TestDuplicateSignatureMemberRejected(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "dup-sig.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin, SignKey: priv})
	if err != nil {
		t.Fatal(err)
	}
	// Append a second SIGNATURE entry with different bytes (zip confusion).
	if err := appendDuplicateZipMember(path, tir.SignatureMemberName, []byte(`{"scheme":"evil"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tir.Verify(path, tir.VerifyOptions{}); !errors.Is(err, tir.ErrTir) {
		t.Fatalf("Verify: want ErrTir for duplicate SIGNATURE, got %v", err)
	}
	if _, err := tir.Load(path); !errors.Is(err, tir.ErrTir) {
		t.Fatalf("Load: want ErrTir for duplicate SIGNATURE, got %v", err)
	}
}

func TestLoadRejectsDuplicateMemberWhenUnsigned(t *testing.T) {
	t.Setenv("TRAJIR_ALLOW_UNVERIFIED", "1")
	// Unsigned Load skips verifySignatureFromZipFiles; duplicate names must
	// still be rejected in loadImpl (CWE-436), not only on the signed path.
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "dup-manifest.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}
	if err := appendDuplicateZipMember(path, "manifest.json", []byte(`{"mode":"thin","evil":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tir.Load(path); !errors.Is(err, tir.ErrTir) {
		t.Fatalf("Load: want ErrTir for duplicate manifest.json, got %v", err)
	}
	if _, err := tir.LoadUnverified(path); !errors.Is(err, tir.ErrTir) {
		t.Fatalf("LoadUnverified: want ErrTir for duplicate manifest.json, got %v", err)
	}
}

func TestSignRejectsInvalidKey(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "k.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}
	if err := tir.Sign(path, ed25519.PrivateKey{1, 2, 3}, tir.SignerMeta{}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("want ErrSignature, got %v", err)
	}
}

func TestSignPreservesPackageMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bit preserve is meaningful on unix mode bits")
	}
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "mode.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	priv := testOnlySeed()
	if err := tir.Sign(path, priv, tir.SignerMeta{ID: "mode"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode after Sign=%v want %v", after.Mode().Perm(), before.Mode().Perm())
	}
}

func TestExportRejectsWrongLengthSignKey(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "seed.tir")
	// 32-byte seed is not a full private key; Export must fail closed (no silent unsigned).
	seed := make(ed25519.PrivateKey, 32)
	_, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin, SignKey: seed})
	if !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("want ErrSignature for 32-byte seed, got %v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatal("export must not leave a package after rejecting SignKey")
	}
}

func TestTrustedKeyIDAllowlist(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "kid.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin, SignKey: priv})
	if err != nil {
		t.Fatal(err)
	}
	kid := tir.KeyID(priv.Public().(ed25519.PublicKey))
	if _, err := tir.Verify(path, tir.VerifyOptions{TrustedKeyIDs: []string{"not-this-key"}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("want key_id trust failure: %v", err)
	}
	if _, err := tir.Verify(path, tir.VerifyOptions{TrustedKeyIDs: []string{kid}}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadUsesSameArchiveForSignature(t *testing.T) {
	// Load must not re-open path for signature: after Load succeeds, Signature
	// matches Verify of the same path, and package nodes remain consistent.
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "same.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{
		Mode:       tir.ModeThin,
		SignKey:    priv,
		SignerMeta: tir.SignerMeta{ID: "same-archive"},
	})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := tir.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Signature == nil || pkg.Signature.SignerID != "same-archive" {
		t.Fatalf("signature=%v", pkg.Signature)
	}
	info, err := tir.Verify(path, tir.VerifyOptions{RequireSignature: true})
	if err != nil {
		t.Fatal(err)
	}
	if info.KeyID != pkg.Signature.KeyID {
		t.Fatalf("key_id Load=%s Verify=%s", pkg.Signature.KeyID, info.KeyID)
	}
	if len(pkg.Nodes) != 5 {
		t.Fatalf("nodes=%d", len(pkg.Nodes))
	}
}

func TestUnknownSchemeFails(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "bad-scheme.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin, SignKey: priv})
	if err != nil {
		t.Fatal(err)
	}
	if err := mutateZipMember(path, tir.SignatureMemberName, func(b []byte) []byte {
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		doc["scheme"] = "not-a-scheme"
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tir.Verify(path, tir.VerifyOptions{}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected scheme error: %v", err)
	}
}

func appendDuplicateZipMember(path, name string, data []byte) error {
	// Read existing entries, rewrite with an extra copy of name (second SIGNATURE).
	type entry struct {
		name string
		data []byte
	}
	var entries []entry
	if err := func() error {
		zr, err := zip.OpenReader(path)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.Name == "" || f.Name[len(f.Name)-1] == '/' {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			body, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return err
			}
			entries = append(entries, entry{name: f.Name, data: body})
		}
		return nil
	}(); err != nil {
		return err
	}
	entries = append(entries, entry{name: name, data: data})

	tmp := path + ".dup"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, err := w.Write(e.data); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func mutateZipMember(path, member string, mut func([]byte) []byte) error {
	type entry struct {
		name string
		data []byte
	}
	var entries []entry

	// Fully close the reader before replacing the file (required on Windows).
	if err := func() error {
		zr, err := zip.OpenReader(path)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.Name == "" || f.Name[len(f.Name)-1] == '/' {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			data, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return err
			}
			if f.Name == member {
				data = mut(data)
			}
			entries = append(entries, entry{name: f.Name, data: data})
		}
		return nil
	}(); err != nil {
		return err
	}

	tmp := path + ".mut"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, err := w.Write(e.data); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func stripZipMember(path, member string) error {
	type entry struct {
		name string
		data []byte
	}
	var entries []entry
	if err := func() error {
		zr, err := zip.OpenReader(path)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if f.Name == "" || f.Name[len(f.Name)-1] == '/' || f.Name == member {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			data, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return err
			}
			entries = append(entries, entry{name: f.Name, data: data})
		}
		return nil
	}(); err != nil {
		return err
	}

	tmp := path + ".strip"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, err := w.Write(e.data); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func TestLoadReaderWithOptions(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	dir := t.TempDir()
	signedPath := filepath.Join(dir, "signed.tir")
	priv := testOnlySeed()
	pub := priv.Public().(ed25519.PublicKey)
	if _, err := tir.Export(src, "t-export", signedPath, tir.ExportOptions{
		Mode:    tir.ModeThin,
		SignKey: priv,
	}); err != nil {
		t.Fatal(err)
	}

	unsignedPath := filepath.Join(dir, "unsigned.tir")
	if _, err := tir.Export(src, "t-export", unsignedPath, tir.ExportOptions{
		Mode: tir.ModeThin,
	}); err != nil {
		t.Fatal(err)
	}

	// 1. Valid signed package with trusted key
	sf, err := os.Open(signedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sf.Close()
	sst, err := sf.Stat()
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := tir.LoadReaderWithOptions(sf, sst.Size(), tir.VerifyOptions{
		TrustedKeys: []ed25519.PublicKey{pub},
	})
	if err != nil {
		t.Fatalf("LoadReaderWithOptions trusted key failed: %v", err)
	}
	if pkg.Signature == nil {
		t.Fatal("expected non-nil Signature")
	}

	// 2. Signed package with untrusted key fails
	otherPub, _, _ := ed25519.GenerateKey(nil)
	if _, err := tir.LoadReaderWithOptions(sf, sst.Size(), tir.VerifyOptions{
		TrustedKeys: []ed25519.PublicKey{otherPub},
	}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for untrusted key, got %v", err)
	}

	// 3. Unsigned package with RequireSignature fails
	uf, err := os.Open(unsignedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer uf.Close()
	ust, err := uf.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tir.LoadReaderWithOptions(uf, ust.Size(), tir.VerifyOptions{
		RequireSignature: true,
	}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for unsigned package with RequireSignature, got %v", err)
	}

	// 4. Unsigned package with empty trust store fails
	if _, err := tir.LoadReaderWithOptions(uf, ust.Size(), tir.VerifyOptions{
		TrustedKeys: []ed25519.PublicKey{},
	}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature for unsigned package with empty TrustedKeys, got %v", err)
	}
}

func TestEmptyTrustStore_RejectsUnsigned(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "unsigned.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}

	// Verify
	if _, err := tir.Verify(path, tir.VerifyOptions{TrustedKeys: []ed25519.PublicKey{}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from Verify with empty TrustedKeys, got %v", err)
	}
	if _, err := tir.Verify(path, tir.VerifyOptions{TrustedKeyIDs: []string{}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from Verify with empty TrustedKeyIDs, got %v", err)
	}

	// LoadWithOptions
	if _, err := tir.LoadWithOptions(path, tir.VerifyOptions{TrustedKeys: []ed25519.PublicKey{}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from LoadWithOptions with empty TrustedKeys, got %v", err)
	}
	if _, err := tir.LoadWithOptions(path, tir.VerifyOptions{TrustedKeyIDs: []string{}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from LoadWithOptions with empty TrustedKeyIDs, got %v", err)
	}

	// ImportWithOptions TrustedKeys
	dest1 := openLog(t, "dest1.sqlite")
	opts1 := tir.ImportOptions{
		Verify: tir.VerifyOptions{
			TrustedKeys: []ed25519.PublicKey{},
		},
	}
	if _, err := tir.ImportWithOptions(path, dest1, opts1); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from ImportWithOptions with empty TrustedKeys, got %v", err)
	}
	nodes1, err := dest1.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes1) != 0 {
		t.Fatalf("expected 0 nodes in dest1, got %d", len(nodes1))
	}

	// ImportWithOptions TrustedKeyIDs
	dest2 := openLog(t, "dest2.sqlite")
	opts2 := tir.ImportOptions{
		Verify: tir.VerifyOptions{
			TrustedKeyIDs: []string{},
		},
	}
	if _, err := tir.ImportWithOptions(path, dest2, opts2); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from ImportWithOptions with empty TrustedKeyIDs, got %v", err)
	}
	nodes2, err := dest2.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes2) != 0 {
		t.Fatalf("expected 0 nodes in dest2, got %d", len(nodes2))
	}
}

func TestEmptyTrustStore_RejectsSigned(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "signed.tir")
	priv := testOnlySeed()
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{
		Mode:    tir.ModeThin,
		SignKey: priv,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify
	if _, err := tir.Verify(path, tir.VerifyOptions{TrustedKeys: []ed25519.PublicKey{}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from Verify with empty TrustedKeys, got %v", err)
	}
	if _, err := tir.Verify(path, tir.VerifyOptions{TrustedKeyIDs: []string{}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from Verify with empty TrustedKeyIDs, got %v", err)
	}

	// LoadWithOptions
	if _, err := tir.LoadWithOptions(path, tir.VerifyOptions{TrustedKeys: []ed25519.PublicKey{}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from LoadWithOptions with empty TrustedKeys, got %v", err)
	}
	if _, err := tir.LoadWithOptions(path, tir.VerifyOptions{TrustedKeyIDs: []string{}}); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from LoadWithOptions with empty TrustedKeyIDs, got %v", err)
	}

	// ImportWithOptions TrustedKeys
	dest1 := openLog(t, "dest_s1.sqlite")
	opts1 := tir.ImportOptions{
		Verify: tir.VerifyOptions{
			TrustedKeys: []ed25519.PublicKey{},
		},
	}
	if _, err := tir.ImportWithOptions(path, dest1, opts1); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from ImportWithOptions with empty TrustedKeys, got %v", err)
	}
	nodes1, err := dest1.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes1) != 0 {
		t.Fatalf("expected 0 nodes in dest1, got %d", len(nodes1))
	}

	// ImportWithOptions TrustedKeyIDs
	dest2 := openLog(t, "dest_s2.sqlite")
	opts2 := tir.ImportOptions{
		Verify: tir.VerifyOptions{
			TrustedKeyIDs: []string{},
		},
	}
	if _, err := tir.ImportWithOptions(path, dest2, opts2); !errors.Is(err, tir.ErrSignature) {
		t.Fatalf("expected ErrSignature from ImportWithOptions with empty TrustedKeyIDs, got %v", err)
	}
	nodes2, err := dest2.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes2) != 0 {
		t.Fatalf("expected 0 nodes in dest2, got %d", len(nodes2))
	}
}
