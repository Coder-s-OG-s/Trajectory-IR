// Command trajir is the local Trajectory IR CLI.
//
// Subcommands:
//
//	verify  offline evidence checks for a .tir package (TURNING_POINT auditor)
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/audit"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "verify":
		os.Exit(cmdVerify(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "trajir: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: trajir <command> [flags]

commands:
  verify   audit a .tir package (hash, seal-before-execute, open-world, signature)

`)
}

type hexKeyFlag struct {
	keys []ed25519.PublicKey
	set  bool
}

func (f *hexKeyFlag) String() string { return "" }

func (f *hexKeyFlag) Set(s string) error {
	raw, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return fmt.Errorf("--trust-key wants %d hex-encoded bytes", ed25519.PublicKeySize)
	}
	f.set = true
	f.keys = append(f.keys, ed25519.PublicKey(raw))
	return nil
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return fmt.Errorf("--trust-key-id is empty")
	}
	*s = append(*s, v)
	return nil
}

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	requireSig := fs.Bool("require-signature", false, "fail when SIGNATURE member is missing")
	asJSON := fs.Bool("json", false, "print Result as JSON")
	var trustKeys hexKeyFlag
	var trustIDs stringList
	fs.Var(&trustKeys, "trust-key", "hex ed25519 public key to trust; repeat for more than one")
	fs.Var(&trustIDs, "trust-key-id", "signer key_id to trust; repeat for more than one")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: trajir verify [--require-signature] [--trust-key hex] [--trust-key-id id] [--json] <pack.tir>\n")
		return 2
	}
	path := fs.Arg(0)
	opts := audit.Options{RequireSignature: *requireSig}
	if trustKeys.set {
		opts.TrustedKeys = trustKeys.keys
	}
	if len(trustIDs) > 0 {
		opts.TrustedKeyIDs = []string(trustIDs)
	}
	res, err := audit.VerifyFile(path, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trajir verify: %v\n", err)
		return 2
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	} else if res.OK {
		fmt.Printf("OK %s\n", path)
	} else {
		fmt.Fprintf(os.Stderr, "FAIL %s\n", path)
		for _, f := range res.Findings {
			fmt.Fprintf(os.Stderr, "  %s\n", f)
		}
	}
	if res.OK {
		return 0
	}
	return 1
}
