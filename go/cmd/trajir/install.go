package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hostinstall"
)

func cmdInstall(args []string, c cliIO) int {
	if len(args) < 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintf(c.stderr, "usage: trajir install <%s> [--target DIR] [--user] [--command CMD] [--dry-run]\n",
			strings.Join(hostinstall.Hosts(), "|"))
		return 2
	}
	host := args[0]
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	target := fs.String("target", "", "project directory (default: current directory)")
	user := fs.Bool("user", false, "write the user-level config instead of the project's")
	command := fs.String("command", "", "command hooks run (default: trajir when it is on PATH, else this binary's path)")
	dry := fs.Bool("dry-run", false, "print the files that would change and write nothing")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *target == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(c.stderr, "trajir install: %v\n", err)
			return 1
		}
		*target = wd
	}
	if *command == "" {
		*command = defaultCommand()
	}
	paths, err := hostinstall.Install(hostinstall.Options{
		Host:    host,
		Target:  *target,
		User:    *user,
		Command: *command,
		DryRun:  *dry,
		Home:    c.getenv("HOME"),
	})
	if err != nil {
		fmt.Fprintf(c.stderr, "trajir install: %v\n", err)
		return 1
	}
	verb := "wrote"
	if *dry {
		verb = "would write"
	}
	for _, p := range paths {
		fmt.Fprintf(c.stdout, "%s %s\n", verb, p)
	}
	fmt.Fprintf(c.stdout, "hooks run: %s hook <phase> --host %s\n", *command, host)
	switch host {
	case "codex":
		fmt.Fprintln(c.stdout, "codex: open /hooks in Codex and trust the trajir hooks before they run")
	case "gemini":
		fmt.Fprintln(c.stdout, "gemini: run /hooks to check the trajir hooks are enabled")
	case "grok":
		fmt.Fprintln(c.stdout, "grok: trust this folder with /hooks-trust or --trust before these hooks run")
	}
	fmt.Fprintln(c.stdout, "dashboard: trajir console, then open http://127.0.0.1:8787/")
	return 0
}

// defaultCommand prefers a trajir on PATH, so config files stay portable.
func defaultCommand() string {
	if _, err := exec.LookPath("trajir"); err == nil {
		return "trajir"
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "trajir"
}
