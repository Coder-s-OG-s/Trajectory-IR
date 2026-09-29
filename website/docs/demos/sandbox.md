# Demo: Sandbox mode (R06)

Demo/CI **effect-class gate**, not a process sandbox. In sandbox mode, tools classified `NON_IDEMPOTENT_WRITE`, `AGENT_SPAWN`, or `SENSITIVE` are rejected **before** the tool body. Classify `bash` as read-only and this gate will believe you. Don't.

Source: same adoption host with `-sandbox`

## Run it yourself

From `go/`:

```bash
go run ./examples/adoption_host -sandbox
```

## Captured output (fixture)

```text
--8<-- "adoption_host_sandbox.txt"
```

## Why show this

- Maps cleanly to “what-if / dry-run” branches in agent systems
- Aligns with MCP-style safety hints, with IR fail-closed defaults when metadata is missing
- Thirty-second stage beat after the crash-resume story
