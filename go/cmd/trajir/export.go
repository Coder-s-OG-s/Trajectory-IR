package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

// workDirFlags picks a run directory: --work-dir, else
// <project>/.trajir/runs/<host> (the hook default).
type workDirFlags struct {
	workDir, project, host *string
}

func addWorkDirFlags(fs *flag.FlagSet) workDirFlags {
	return workDirFlags{
		workDir: fs.String("work-dir", "", "run directory holding nodes.sqlite"),
		project: fs.String("project", "", "project directory (default: current directory)"),
		host:    fs.String("host", "", "agent host whose runs to use, for example claude"),
	}
}

func (w workDirFlags) projectDir() (string, error) {
	if *w.project != "" {
		return *w.project, nil
	}
	return os.Getwd()
}

func (w workDirFlags) resolve() (string, error) {
	if *w.workDir != "" {
		return *w.workDir, nil
	}
	if *w.host == "" {
		return "", fmt.Errorf("pass --host or --work-dir")
	}
	proj, err := w.projectDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(proj, ".trajir", "runs", *w.host), nil
}

func cmdExport(args []string, c cliIO) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	wd := addWorkDirFlags(fs)
	traj := fs.String("trajectory", "", "trajectory id (see trajir runs)")
	tenant := fs.String("tenant", "local", "tenant id")
	mode := fs.String("mode", "thin", "package mode: thin or fat")
	out := fs.String("o", "", "output .tir path (default <trajectory>.tir in the current directory)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *traj == "" {
		fmt.Fprintln(c.stderr, "usage: trajir export (--host NAME [--project DIR] | --work-dir DIR) --trajectory ID [-o out.tir]")
		return 2
	}
	var m tir.Mode
	switch strings.ToLower(*mode) {
	case "thin":
		m = tir.ModeThin
	case "fat":
		m = tir.ModeFat
	default:
		fmt.Fprintf(c.stderr, "trajir export: unsupported mode %q (use thin or fat)\n", *mode)
		return 2
	}
	dir, err := wd.resolve()
	if err != nil {
		fmt.Fprintf(c.stderr, "trajir export: %v\n", err)
		return 2
	}
	dest := *out
	if dest == "" {
		dest = *traj + ".tir"
	}
	nl, err := openRunLog(dir)
	if err != nil {
		fmt.Fprintf(c.stderr, "trajir export: %v\n", err)
		return 1
	}
	defer nl.Close()
	ok, err := nl.HasTrajectory(*traj, *tenant)
	if err != nil || !ok {
		fmt.Fprintf(c.stderr, "trajir export: no trajectory %q for tenant %q in %s\n", *traj, *tenant, dir)
		return 1
	}
	sink, _ := hosthook.DefaultSink(c.getenv)
	path, err := tir.Export(nl, *traj, dest, tir.ExportOptions{
		Mode:     m,
		TenantID: tenant,
		OnExported: func(n tir.ExportNotice) {
			redacted, okN := n.Redacted, n.OK
			emit.NotePackage(sink, emit.PackageFact{
				Kind:         emit.KindExportCompleted,
				TrajectoryID: *traj,
				TenantID:     *tenant,
				Path:         n.Path,
				Mode:         n.Mode,
				Redacted:     &redacted,
				Bytes:        n.Bytes,
				MemberCount:  n.MemberCount,
				NodeCount:    n.NodeCount,
				OK:           &okN,
				Error:        n.Error,
				Source:       "cli",
				Runtime:      "trajir-export",
			})
		},
	})
	if err != nil {
		fmt.Fprintf(c.stderr, "trajir export: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.stdout, "%s\n", path)
	return 0
}

func openRunLog(dir string) (*nodelog.NodeLog, error) {
	path := filepath.Join(dir, "nodes.sqlite")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no recorded runs at %s", path)
	}
	return nodelog.Open(path)
}

func cmdRuns(args []string, c cliIO) int {
	fs := flag.NewFlagSet("runs", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	wd := addWorkDirFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var dirs []string
	if *wd.workDir != "" || *wd.host != "" {
		dir, err := wd.resolve()
		if err != nil {
			fmt.Fprintf(c.stderr, "trajir runs: %v\n", err)
			return 2
		}
		dirs = []string{dir}
	} else {
		proj, err := wd.projectDir()
		if err != nil {
			fmt.Fprintf(c.stderr, "trajir runs: %v\n", err)
			return 2
		}
		dirs, _ = filepath.Glob(filepath.Join(proj, ".trajir", "runs", "*"))
		sort.Strings(dirs)
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 2, 2, ' ', 0)
	found := 0
	for _, dir := range dirs {
		nl, err := openRunLog(dir)
		if err != nil {
			continue
		}
		list, err := nl.ListTrajectories()
		_ = nl.Close()
		if err != nil {
			fmt.Fprintf(c.stderr, "trajir runs: %s: %v\n", dir, err)
			continue
		}
		for _, t := range list {
			if found == 0 {
				fmt.Fprintln(tw, "HOST\tTRAJECTORY\tTENANT\tSTEPS\tNODES\tLAST")
			}
			found++
			last := time.Unix(int64(t.LastTS), 0).Local().Format("2006-01-02 15:04")
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\n", filepath.Base(dir), t.TrajectoryID, t.TenantID, t.SealCount, t.NodeCount, last)
		}
	}
	_ = tw.Flush()
	if found == 0 {
		fmt.Fprintln(c.stdout, "no runs recorded yet")
	}
	return 0
}
