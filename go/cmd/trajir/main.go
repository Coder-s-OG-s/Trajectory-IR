// Command trajir is the local Trajectory IR CLI.
//
// Subcommands:
//
//	hook     record an agent host's tool call (called by host hooks)
//	install  write hook and MCP config for a host into a project
//	runs     list recorded sessions
//	export   export one session as a .tir package
//	verify   offline evidence checks for a .tir package
//	console  serve the dashboard
//	mcp      run the MCP server on stdio
//	version  print the version
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/audit"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

// version is set at release time with -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

type cliIO struct {
	stdin          io.Reader
	stdout, stderr io.Writer
	getenv         func(string) string
}

func main() {
	os.Exit(run(os.Args[1:], cliIO{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv}))
}

func run(args []string, c cliIO) int {
	if len(args) < 1 {
		usage(c.stderr)
		return 2
	}
	switch args[0] {
	case "hook":
		return cmdHook(args[1:], c)
	case "install":
		return cmdInstall(args[1:], c)
	case "runs":
		return cmdRuns(args[1:], c)
	case "export":
		return cmdExport(args[1:], c)
	case "verify":
		return cmdVerify(args[1:], c)
	case "console":
		return cmdConsole(args[1:], c)
	case "mcp":
		return cmdMCP(args[1:], c)
	case "version", "--version":
		fmt.Fprintf(c.stdout, "trajir %s\n", version)
		return 0
	case "help", "-h", "--help":
		usage(c.stderr)
		return 0
	default:
		fmt.Fprintf(c.stderr, "trajir: unknown command %q\n", args[0])
		usage(c.stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `usage: trajir <command> [flags]

commands:
  hook     record a tool call from an agent host (pre, post, post-failure, session-start)
  install  write hook and MCP config for codex, cursor, copilot, gemini or claude
  runs     list recorded agent sessions in a project
  export   export one session as a .tir package
  verify   audit a .tir package (hash, seal-before-execute, open-world, signature)
  console  serve the dashboard on 127.0.0.1:8787
  mcp      run the Trajectory IR MCP server on stdio
  version  print the version

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

func cmdVerify(args []string, c cliIO) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
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
		fmt.Fprintf(c.stderr, "usage: trajir verify [--require-signature] [--trust-key hex] [--trust-key-id id] [--json] <pack.tir>\n")
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
		fmt.Fprintf(c.stderr, "trajir verify: %v\n", err)
		return 2
	}
	noteAudit(c, path, res)
	if *asJSON {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	} else if res.OK {
		fmt.Fprintf(c.stdout, "OK %s\n", path)
	} else {
		fmt.Fprintf(c.stderr, "FAIL %s\n", path)
		for _, f := range res.Findings {
			fmt.Fprintf(c.stderr, "  %s\n", f)
		}
	}
	if res.OK {
		return 0
	}
	return 1
}

// noteAudit sends audit.completed to the dashboard. The trajectory id
// comes from the package manifest. A package that does not load is not
// reported, because there is no run to file it under.
func noteAudit(c cliIO, path string, res *audit.Result) {
	sink, _ := hosthook.DefaultSink(c.getenv)
	if sink == nil {
		return
	}
	pkg, err := tir.Load(path)
	if err != nil {
		return
	}
	traj, _ := pkg.Manifest["trajectory_id"].(string)
	tenant, _ := pkg.Manifest["tenant_id"].(string)
	if traj == "" {
		return
	}
	findings := make([]any, 0, len(res.Findings))
	for _, f := range res.Findings {
		findings = append(findings, f)
	}
	emit.NoteAudit(sink, emit.AuditFact{
		TrajectoryID: traj,
		TenantID:     tenant,
		Path:         path,
		OK:           res.OK,
		Findings:     findings,
		Source:       "cli",
		Runtime:      "trajir-verify",
	})
}
