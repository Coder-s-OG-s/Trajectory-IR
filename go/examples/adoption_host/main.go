package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/sandbox"
)

func main() {
	sandboxFlag := flag.Bool("sandbox", false, "reject NON_IDEMPOTENT_WRITE tools")
	withPackage := flag.Bool("with-package", false, "after live step, CAS put + thin .tir + rehydrate")
	workDir := flag.String("workdir", "", "directory for sqlite files (default: temp)")
	casRoot := flag.String("cas-root", "", "CAS root when using -with-package (default: temp)")
	tirOut := flag.String("tir-out", "", "destination .tir when using -with-package (default: temp)")
	flag.Parse()

	if *sandboxFlag && *withPackage {
		fmt.Fprintln(os.Stderr, "error: -sandbox and -with-package cannot be combined")
		os.Exit(2)
	}

	dir := *workDir
	cleanupDir := false
	if dir == "" {
		var err error
		dir, err = defaultWorkDir()
		if err != nil {
			fail(err)
		}
		cleanupDir = true
	}
	if err := ensureDir(dir); err != nil {
		fail(err)
	}
	if cleanupDir {
		defer os.RemoveAll(dir)
	}

	if *sandboxFlag {
		_, err := runHostStep(dir, true, nil)
		var forbidden *sandbox.Forbidden
		if errors.As(err, &forbidden) {
			fmt.Printf("sandbox correctly rejected non idempotent tool: %v\n", forbidden)
			os.Exit(0)
		}
		if err != nil {
			fail(err)
		}
		fmt.Fprintln(os.Stderr, "error: sandbox should have rejected ship_release")
		os.Exit(1)
	}

	step, err := runHostStep(dir, false, nil)
	if err != nil {
		fail(err)
	}
	fmt.Println("adoption host step complete")
	fmt.Printf("  tool results: %v\n", step.ToolResults)
	fmt.Printf("  node kinds:   %v\n", step.NodeKinds)

	if *withPackage {
		casDir := *casRoot
		if casDir == "" {
			casDir = absJoin(dir, "cas")
		}
		dest := *tirOut
		if dest == "" {
			dest = absJoin(dir, "adoption-run.tir")
		}
		payload, err := reportBytes(step)
		if err != nil {
			fail(err)
		}
		pkg, err := exportThinPackage(dir, casDir, dest, payload)
		if err != nil {
			fail(err)
		}
		fmt.Println("thin package path complete")
		fmt.Printf("  tir:          %s\n", pkg.TirPath)
		fmt.Printf("  content_hash: %s\n", pkg.ContentHash)
		fmt.Printf("  logical_path: %s\n", pkg.LogicalPath)
		fmt.Printf("  node_count:   %d\n", pkg.NodeCount)
		fmt.Printf("  rehydrate:    %d bytes ok\n", len(pkg.Rehydrated))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
