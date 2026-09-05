# Conference pitch (stable wording)

## One sentence

Trajectory IR is a portable, hash-verifiable intermediate representation for agent execution trajectories (typed nodes, sealed decisions, effect classes, thin/fat `.tir` packages) that runs **on top of** existing durable execution backends — not a replacement for Temporal, DBOS, or agent frameworks.

## What the room should remember

| Message | Proof in demo |
|---|---|
| Cloud-native fit | Agents on K8s/cloud need portable audit/export across runtimes |
| Novel gap | Runtime-independent, effect-safe unit of “what the agent did” |
| Non-overlap | Engines own crash/retry; we own seals + portability |
| Spec-shaped | Conformance R01–R08; dual SDK (Go primary / Python reference) |

## CNCF Sandbox wording (do not improvise)

Use:

> We are an Apache 2.0 open source project preparing a CNCF Sandbox application. We are not a CNCF project until the TOC approves and the contribution agreement is signed.

Avoid:

- “We are in CNCF Sandbox”
- “Donated to the CNCF”
- “Official CNCF project”

Checklist and form map: [docs/CNCF_SANDBOX_APPLICATION_OUTLINE.md](https://github.com/Coder-s-OG-s/Trajectory-IR/blob/main/docs/CNCF_SANDBOX_APPLICATION_OUTLINE.md).
