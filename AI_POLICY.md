# AI Usage Policy

This policy governs the use of AI coding assistants (e.g. Claude Code, Antigravity IDE, GitHub Copilot, ChatGPT, or any other AI/LLM-based tool) when contributing to Trajectory IR. It is binding on all contributors and complements [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and the [Code of Conduct](CODE_OF_CONDUCT.md).

## 1. Why this exists

Trajectory IR is infrastructure for autonomous AI agents (see [SECURITY.md](SECURITY.md)). AI assistance in writing that infrastructure is welcome, but it must not be used as a way to offload judgment, accountability, or review effort onto a tool that has no stake in the project's correctness or safety.

## 2. Disclosure

If you use AI tools for a meaningful share of a change, say so in the pull request description. This is already required by [CONTRIBUTING.md §3](CONTRIBUTING.md#3-pull-requests). Undisclosed, substantially AI-generated contributions are treated as a policy violation, not merely an oversight.

## 3. Accountability

As stated in [SECURITY.md §4](SECURITY.md#4-security-accountability-for-contributors): if you use an AI coding assistant to draft a PR, **you, the human contributor, are 100% accountable** for everything it introduces — correctness, security, licensing, and spec compliance included. AI agents have no built-in trust regarding security boundaries and "the AI wrote it" is never an acceptable explanation for a defect, a spec deviation, or a licensing problem.

## 4. Rules for AI-assisted contributions

1. **The spec governs, not general AI knowledge.** Per the root [README.md §0 and §15](README.md), AI agents (and contributors using them) must implement exactly what the spec defines. Do not let a tool invent behavior from familiarity with similar systems. Undefined behavior gets a `SPEC-QUESTION` issue, not an improvisation.
2. **No AI-generated content that fabricates authorship, test results, benchmarks, or provenance.** Generated code, docs, and commit messages must accurately represent what was actually done and verified.
3. **No bypassing review or CI gates using AI tooling.** Mandatory human sign-off on `pkg/effects/`, `pkg/resume/`, and their Go equivalents (per [SECURITY.md §4.2](SECURITY.md#4-security-accountability-for-contributors)) applies regardless of how the change was produced.
4. **No submission of AI output you have not read and verified.** Pasting unreviewed model output into a PR, issue, or review comment is not a contribution.
5. **Respect licensing.** Do not submit AI-generated code that reproduces another project's copyrighted or incompatibly-licensed code, whether or not the tool disclosed the source.
6. **Do not use AI tools to harass, spam, or flood** maintainers or other contributors (mass-generated low-effort issues/PRs, auto-submitted review responses, etc.). This is also a [Code of Conduct](CODE_OF_CONDUCT.md) violation.

## 5. Enforcement

Violations of this policy are handled progressively:

1. **First warning** — a maintainer flags the violation on the relevant issue/PR and explains the required fix.
2. **Second warning** — a further violation (same or different rule) results in a second, formal warning recorded by a maintainer.
3. **Third warning** — a third violation results in a final warning stating that any further violation will result in a ban.
4. **Permanent ban** — a violation after the third warning results in the contributor being permanently banned from the project (repository access revoked, future contributions rejected).

Maintainers may skip directly to a ban, at their discretion, for severe or malicious violations (e.g. deliberately submitting harmful code, large-scale spam, or fabricated security/vulnerability reports), consistent with the enforcement discretion described in the [Code of Conduct](CODE_OF_CONDUCT.md).

Warnings are issued and tracked by maintainers on the relevant GitHub issue/PR thread. A contributor may appeal a warning or ban by emailing the maintainers (below).

## 6. Questions and appeals

For questions about this policy, or to appeal a warning or ban, email the lead maintainers directly:

- `siddharthagithub0007@gmail.com`
- `ayushpatel2731@gmail.com`

Do not use public GitHub issues for appeals of an enforcement decision; email keeps that discussion private, consistent with the reporting channel already used for security issues in [SECURITY.md](SECURITY.md).

---

*This policy may be revised by maintainer consensus per [GOVERNANCE.md](GOVERNANCE.md). See [MAINTAINERS.md](MAINTAINERS.md) for the current maintainer list.*
