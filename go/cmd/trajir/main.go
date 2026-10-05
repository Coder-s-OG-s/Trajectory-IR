// Command trajir is the local Trajectory IR CLI.
//
// Subcommands:
//
//	verify  offline evidence checks for a .tir package (TURNING_POINT auditor)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

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

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	requireSig := fs.Bool("require-signature", false, "fail when SIGNATURE member is missing")
	asJSON := fs.Bool("json", false, "print Result as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: trajir verify [--require-signature] [--json] <pack.tir>\n")
		return 2
	}
	path := fs.Arg(0)
	res, err := audit.VerifyFile(path, audit.Options{RequireSignature: *requireSig})
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
