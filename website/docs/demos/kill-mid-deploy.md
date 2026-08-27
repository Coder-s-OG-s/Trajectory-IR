# Demo: Kill mid deploy

**The hero demo.** Shows why Trajectory IR exists: a non-idempotent tool crashes mid-flight, and resume must not silently re-run the side effect or re-ask the model after the decision was sealed.

Source: [`go/examples/kill_mid_deploy`](https://github.com/Coder-s-OG-s/Trajectory-IR/tree/main/go/examples/kill_mid_deploy)

## Story in one minute

1. Model produces a deploy plan → **DECISION sealed**
2. `deploy_server` (`NON_IDEMPOTENT_WRITE`) starts
3. Process is killed mid-tool
4. On `-resume`: model is **not** called again; deploy is **not** blindly retried → `BLOCKED_NEEDS_GATE`
5. Counters prove it: `model_count=1`, `deploy_count=0`

## Run it yourself

From `go/`:

=== "First run (then kill)"

    ```bash
    go run ./examples/kill_mid_deploy \
      -workdir ./kill_mid_deploy-data \
      -crash-during=tool_call
    ```

    When you see `TOOL_CALL: deploy_server started`, kill the process (`Ctrl+C`, `kill -9`, or `Stop-Process` on Windows).

=== "Resume"

    ```bash
    go run ./examples/kill_mid_deploy \
      -workdir ./kill_mid_deploy-data \
      -resume
    ```

## Captured output (fixture)

### First run

```text
--8<-- "kill_mid_deploy_first.txt"
```

### Resume after kill

```text
--8<-- "kill_mid_deploy_resume.txt"
```

!!! success "What the audience should hear"
    Durable engines keep the workflow alive. Trajectory IR makes the **agent decision and tool safety** honest across that resume: seals freeze the plan; block-and-gate stops a duplicate deploy.

## Alternate mode (R01 style)

Crash **after** seal, before tools complete:

```bash
go run ./examples/kill_mid_deploy \
  -workdir ./kill_mid_deploy-data2 \
  -crash-after=decision_sealed
# kill after DECISION sealed
go run ./examples/kill_mid_deploy \
  -workdir ./kill_mid_deploy-data2 \
  -resume
```

Expect `model_count=1` across both runs and a completed deploy on resume (`deploy_count=1`).
