// Command crashagent is a fixture for R01/R02 style crash resume tests.
//
//	crashagent -workdir DIR -crash-after=decision_sealed
//	crashagent -workdir DIR -resume
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

const (
	tenantID     = "demo"
	trajectoryID = "go-crash-demo"
	workflowID   = "go-crash-demo-wf"
	crashWindow  = 30 * time.Second
)

func main() {
	workdir := flag.String("workdir", ".", "directory for sqlite files, counters, markers")
	crashAfter := flag.String("crash-after", "", "decision_sealed")
	crashDuring := flag.String("crash-during", "", "tool_call")
	resumeMode := flag.Bool("resume", false, "reopen same workdir and continue")
	flag.Parse()

	if err := os.Chdir(*workdir); err != nil {
		fail(err)
	}

	logPath := "nodes.sqlite"
	memoPath := "memo.sqlite"

	nl, err := nodelog.Open(logPath)
	if err != nil {
		fail(err)
	}
	defer nl.Close()

	backend, err := durable.OpenLocal(memoPath)
	if err != nil {
		fail(err)
	}
	defer backend.Close()

	crashDuringTool := strings.EqualFold(*crashDuring, "tool_call")
	crashAfterSeal := strings.EqualFold(*crashAfter, "decision_sealed")

	cfg := resume.RunStepConfig{
		Log:          nl,
		Backend:      backend,
		TenantID:     tenantID,
		TrajectoryID: trajectoryID,
		WorkflowID:   workflowID,
		Tools: map[string]resume.Tool{
			"deploy_server": {
				Name:   "deploy_server",
				Effect: effects.NON_IDEMPOTENT_WRITE,
				Fn: func(args map[string]any) (any, error) {
					if err := writeFile("tool_started.marker", "started"); err != nil {
						return nil, err
					}
					fmt.Println("TOOL_CALL: deploy_server started")
					if crashDuringTool && !*resumeMode {
						time.Sleep(crashWindow)
					}
					if _, err := bumpCounter("test_deploy_side_effect_count.txt"); err != nil {
						return nil, err
					}
					return map[string]any{"deployed": args["version"]}, nil
				},
			},
		},
		OnDecisionSealed: func() {
			_ = writeFile("decision_sealed.marker", "sealed")
			fmt.Println("DECISION sealed")
			if crashAfterSeal && !*resumeMode {
				time.Sleep(crashWindow)
			}
		},
	}

	model := func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		if _, err := bumpCounter("test_model_call_count.txt"); err != nil {
			return nil, err
		}
		fmt.Println("INFERENCE: model_call invoked")
		return map[string]any{
			"tool_calls": []any{
				map[string]any{
					"name": "deploy_server",
					"args": map[string]any{"version": "1.0.0"},
				},
			},
		}, nil
	}

	ctx := context.Background()
	_, err = resume.RunStep(ctx, cfg, 1, model, map[string]any{"run": "crashagent"})
	if err != nil {
		var blocked *resume.BlockedNeedsGate
		if errors.As(err, &blocked) {
			fmt.Printf("BLOCKED_NEEDS_GATE: %s\n", blocked.Error())
			os.Exit(0)
		}
		fail(err)
	}
	fmt.Println("step committed")
}

func bumpCounter(path string) (int, error) {
	n := 0
	if b, err := os.ReadFile(path); err == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	n++
	return n, writeFile(path, strconv.Itoa(n))
}

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
