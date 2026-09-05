# Demo: Sandbox mode (R06)

Fast safety demo. In sandbox mode, real `NON_IDEMPOTENT_WRITE` tools are rejected **before** side effects.

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
