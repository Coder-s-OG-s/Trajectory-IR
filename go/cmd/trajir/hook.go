package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook/hosts"
)

// maxPayload bounds what a hook reads from stdin.
const maxPayload = 256 << 20

// cmdHook is what host hooks run: trajir hook <phase> --host <name>.
//
// pre fails closed: anything that stops the seal blocks the tool, in the
// host's own block format. post, post-failure and session-start fail open:
// they print to stderr and exit 0, so a broken recorder never asks the host
// to run a tool again.
func cmdHook(args []string, c cliIO) int {
	if len(args) < 1 {
		fmt.Fprintln(c.stderr, "usage: trajir hook <pre|post|post-failure|session-start> --host <name>")
		return 2
	}
	phase := hosthook.Phase(args[0])
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	host := fs.String("host", "", "agent host: "+fmt.Sprint(hosts.Names()))
	owner := fs.String("owner", "", "names this hook registration (plugin, project, user) so two registrations of one call are not a replay")
	if err := fs.Parse(args[1:]); err != nil {
		return failCode(phase)
	}
	switch phase {
	case hosthook.PhasePre, hosthook.PhasePost, hosthook.PhasePostFailure, hosthook.PhaseSessionStart:
	default:
		fmt.Fprintf(c.stderr, "trajir hook: unknown phase %q\n", phase)
		return 2
	}
	adapter, err := hosts.Lookup(*host)
	if err != nil {
		fmt.Fprintf(c.stderr, "trajir hook: %v\n", err)
		return failCode(phase)
	}
	raw, err := io.ReadAll(io.LimitReader(c.stdin, maxPayload))
	if err != nil {
		return fail(c, adapter, phase, "could not read the hook payload: "+err.Error())
	}
	ev, err := adapter.Parse(phase, raw)
	if err != nil {
		return fail(c, adapter, phase, err.Error())
	}
	id, err := hosthook.ResolveIdentity(adapter.Name(), ev, c.getenv)
	if err != nil {
		return fail(c, adapter, phase, err.Error())
	}
	if err := id.Prepare(); err != nil {
		return fail(c, adapter, phase, err.Error())
	}
	sink, consoleDir := hosthook.DefaultSink(c.getenv)
	s := hosthook.Session{
		WorkDir:      id.WorkDir,
		TenantID:     id.TenantID,
		TrajectoryID: id.TrajectoryID,
		Owner:        hookOwner(*owner, c.getenv),
		Sink:         sink,
		Runtime:      adapter.Runtime(),
	}
	switch phase {
	case hosthook.PhaseSessionStart:
		where := "dashboard events off"
		if consoleDir != "" {
			where = "dashboard data in " + consoleDir
		}
		fmt.Fprintf(c.stderr, "trajir: recording %s tool calls to %s (%s); run `trajir console` to see them\n",
			adapter.Name(), id.WorkDir, where)
		return 0
	case hosthook.PhasePre:
		classify, err := hosthook.NewClassifier(adapter.Name(), id.ProjectDir)
		if err != nil {
			return fail(c, adapter, phase, err.Error())
		}
		s.Classify = classify
		rec, err := s.SealBeforeTool(ev.ToolName, ev.Args, ev.ToolUseID, ev.Annotations)
		if err != nil {
			return fail(c, adapter, phase, fmt.Sprintf("could not seal %s: %v", ev.ToolName, err))
		}
		if hosthook.ReplayBlocked(rec) {
			return adapter.Block(c.stdout, c.stderr, fmt.Sprintf(
				"%s was already sealed as step %d of this session and is not run again without a person (block-and-gate)",
				rec.ToolName, rec.StepN))
		}
		return 0
	default:
		if err := record(s, phase, ev); err != nil {
			if errors.Is(err, hosthook.ErrNoSeal) {
				fmt.Fprintf(c.stderr, "trajir hook: %v; nothing recorded\n", err)
			} else {
				fmt.Fprintf(c.stderr, "trajir hook: %v\n", err)
			}
		}
		return 0
	}
}

func record(s hosthook.Session, phase hosthook.Phase, ev hosthook.Event) error {
	failed := phase == hosthook.PhasePostFailure || ev.Failed
	var err error
	switch {
	case failed && ev.ToolUseID != "":
		_, err = s.RecordToolFailure(ev.ToolUseID, ev.Error)
	case failed:
		_, err = s.RecordMatchedFailure(ev.ToolName, ev.Args, ev.Error)
	case ev.ToolUseID != "":
		_, err = s.RecordToolResult(ev.ToolUseID, ev.Result)
	default:
		_, err = s.RecordMatchedResult(ev.ToolName, ev.Args, ev.Result)
	}
	return err
}

// hookOwner is --owner, else TRAJIR_HOOK_OWNER (the Python hooks' variable).
func hookOwner(flagValue string, getenv func(string) string) string {
	if flagValue != "" {
		return flagValue
	}
	return getenv("TRAJIR_HOOK_OWNER")
}

func failCode(phase hosthook.Phase) int {
	if phase == hosthook.PhasePre {
		return 2
	}
	return 0
}

// fail blocks on pre and only reports on the other phases.
func fail(c cliIO, a hosts.Adapter, phase hosthook.Phase, reason string) int {
	if phase == hosthook.PhasePre {
		return a.Block(c.stdout, c.stderr, "tool call not recorded, so it is blocked: "+reason)
	}
	fmt.Fprintf(c.stderr, "trajir hook: %s\n", reason)
	return 0
}
