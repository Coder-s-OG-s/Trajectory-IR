// Command kill-mid-deploy is the human runnable Go demo for honest seal resume.
//
//	go run ./examples/kill-mid-deploy -workdir ./demo-run -crash-during=tool_call
//	# kill the process when you see the tool started line
//	go run ./examples/kill-mid-deploy -workdir ./demo-run -resume
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

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

const (
	tenantID     = "demo"
	trajectoryID = "go-kill-mid-deploy"
	workflowID   = "go-kill-mid-deploy-wf"
	crashWindow  = 45 * time.Second
)

func main() {
	workdir := flag.String("workdir", "./kill-mid-deploy-data", "directory for sqlite files and counters")
	crashAfter := flag.String("crash-after", "", "decision_sealed: idle after DECISION so you can kill")
	crashDuring := flag.String("crash-during", "", "tool_call: idle inside deploy so you can kill")
	resumeMode := flag.Bool("resume", false, "reopen the same workdir and continue")
	flag.Parse()

	if err := os.MkdirAll(*workdir, 0o755); err != nil {
		fail(err)
	}

	fmt.Println("Trajectory IR Go demo: kill mid deploy")
	fmt.Printf("workdir: %s\n", *workdir)
	if *resumeMode {
		fmt.Println("mode: resume")
	} else {
		fmt.Println("mode: first run")
	}

	tr, err := openHandle(*workdir, *resumeMode)
	if err != nil {
		fail(err)
	}
	defer tr.Close()

	crashDuringTool := strings.EqualFold(*crashDuring, "tool_call")
	crashAfterSeal := strings.EqualFold(*crashAfter, "decision_sealed")

	tools := map[string]resume.Tool{
		"deploy_server": {
			Name:   "deploy_server",
			Effect: effects.NON_IDEMPOTENT_WRITE,
			Fn: func(args map[string]any) (any, error) {
				path := counterPath(*workdir, "tool_started.marker")
				if err := os.WriteFile(path, []byte("started"), 0o644); err != nil {
					return nil, err
				}
				fmt.Println("TOOL_CALL: deploy_server started")
				fmt.Println(">>> Kill this process now (Ctrl+C or kill -9 / taskkill), then re-run with -resume")
				if crashDuringTool && !*resumeMode {
					time.Sleep(crashWindow)
				}
				if _, err := bumpCounter(counterPath(*workdir, "deploy_count.txt")); err != nil {
					return nil, err
				}
				fmt.Println("TOOL_RESULT: deploy finished")
				return map[string]any{"deployed": args["version"]}, nil
			},
		},
	}

	model := func(ctx context.Context, _ map[string]any) (map[string]any, error) {
		if _, err := bumpCounter(counterPath(*workdir, "model_count.txt")); err != nil {
			return nil, err
		}
		fmt.Println("INFERENCE: model produced a deploy plan")
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
	_, err = tr.RunStep(ctx, 1, model, tools, map[string]any{"demo": "kill-mid-deploy"}, client.RunStepOpts{
		OnDecisionSealed: func() {
			path := counterPath(*workdir, "decision_sealed.marker")
			_ = os.WriteFile(path, []byte("sealed"), 0o644)
			fmt.Println("DECISION sealed (plan frozen; resume must not re-ask the model for this step)")
			if crashAfterSeal && !*resumeMode {
				fmt.Println(">>> Kill this process now, then re-run with -resume")
				time.Sleep(crashWindow)
			}
		},
	})
	if err != nil {
		var blocked *resume.BlockedNeedsGate
		if errors.As(err, &blocked) {
			fmt.Printf("BLOCKED_NEEDS_GATE: %s\n", blocked.Error())
			fmt.Println("Honest resume: non idempotent deploy was not silently re-run.")
			printCounters(*workdir)
			os.Exit(0)
		}
		fail(err)
	}

	if *resumeMode {
		fmt.Println("Resumed. Step committed.")
	} else {
		fmt.Println("Step committed (no kill).")
	}
	printCounters(*workdir)
}

func openHandle(workdir string, resumeMode bool) (*client.Trajectory, error) {
	opts := client.Options{
		WorkDir:    workdir,
		WorkflowID: workflowID,
	}
	if resumeMode {
		return client.Resume(tenantID, trajectoryID, opts)
	}
	return client.OpenTrajectory(tenantID, trajectoryID, opts)
}

func counterPath(workdir, name string) string {
	return workdir + string(os.PathSeparator) + name
}

func bumpCounter(path string) (int, error) {
	n := 0
	if b, err := os.ReadFile(path); err == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	n++
	return n, os.WriteFile(path, []byte(strconv.Itoa(n)), 0o644)
}

func printCounters(workdir string) {
	fmt.Printf("model_count=%d deploy_count=%d\n",
		readCounter(counterPath(workdir, "model_count.txt")),
		readCounter(counterPath(workdir, "deploy_count.txt")),
	)
}

func readCounter(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
