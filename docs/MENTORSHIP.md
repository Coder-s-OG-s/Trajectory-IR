# Mentorship project ideas

Project ideas sized for one mentee over about 12 weeks. We are not enrolled in
LFX Mentorship or any other program yet (see
[#444](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/444)). This page
exists so the ideas, mentors and prerequisites are ready when we are.

Anyone can also pick one of these up outside a program. Comment on the
tracking issue first.

## Before you apply

1. Run the [Go quickstart](../go/QUICKSTART.md) end to end.
2. Read [SCOPE_AND_NON_GOALS.md](SCOPE_AND_NON_GOALS.md) and skim
   [MASTER_SPECIFICATION.md](MASTER_SPECIFICATION.md).
3. Land one merged PR from the
   [good first issue](https://github.com/Coder-s-OG-s/Trajectory-IR/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22)
   list. DCO sign-off is required, see [CONTRIBUTING.md](../CONTRIBUTING.md).

## Ideas

### 1. Restate durable backend for the Go SDK

Go ships Temporal, SQLite and in-memory backends under `go/trajir/durable`.
Python already has a Restate adapter. Add the Go one behind the same
`Backend` interface.

- Outcome: `go/trajir/durable/restate`, conformance R01 and R02 green against it, a short doc
- Skills: Go, some durable execution background
- Tracking issue: to be filed
- Rule: adapter only. No custom retry, lease or crash logic.

### 2. Console seal timeline and integrity view

Operators check seals from logs or by unpacking `.tir` files today. Build the
Seals view described in
[#395](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/395), part of the
console epic [#391](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/391).

- Outcome: timeline of seal events per trajectory, failed verify shown with its reason
- Skills: Go, frontend basics, reading an event spec ([CONSOLE_EVENTS.md](CONSOLE_EVENTS.md))
- Tracking issue: #395

### 3. Cross-runtime `.tir` interop demo

The roadmap lists a documented path for producing a `.tir` in one runtime and
consuming it in another. Build that as a runnable example: export from a Go
host on Temporal, import and verify in the Python reference on DBOS, and the
other way round.

- Outcome: example under `go/examples` and `examples/`, CI job that runs both directions, a walkthrough doc
- Skills: Go and Python, Docker
- Tracking issue: to be filed

## Mentors

Maintainers listed in [MAINTAINERS.md](../MAINTAINERS.md). Each idea gets a
named primary mentor when a term is proposed.
