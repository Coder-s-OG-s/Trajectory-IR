# Security Policy for Trajectory IR

Trajectory IR acts as the durable semantic and execution layer for autonomous AI agents. A compromise in this layer could lead to unauthorized tool execution, state manipulation, or the leakage of sensitive data (like PII or secrets). 

This policy is tightly integrated with our [Infrastructure Design](infrastructure.md), [Contributing Guidelines](CONTRIBUTING.md), and [Code of Conduct](CODE_OF_CONDUCT.md).

## 1. Supported Versions

Trajectory IR is currently in its **Phase 1A / v0.1.x** development cycle. 

| Version | Supported          | Notes |
| ------- | ------------------ | ----- |
| v0.1.x  | :white_check_mark: | Active development (DBOS embedded backend) |
| < v0.1  | :x:                | Historical (CAMI/CLOOP prototypes) |

## 2. Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability, please send an e-mail to the core maintainers: `siddharthagithub0007@gmail.com` *(or project owner email)*.

We will acknowledge receipt of your vulnerability report within 48 hours. Please adhere to the [Code of Conduct](CODE_OF_CONDUCT.md) during this process—public zero-day drops or harassment of maintainers over patches are strict violations of our community standards.

## 3. Scope of Security Concerns (Architecture Specific)

Based on the [Infrastructure Blueprint](infrastructure.md) and [Master Spec](README.md), we are actively monitoring for vulnerabilities in the following planes:

### A. Execution & Tool Safety Plane
- **Safety Boundary Bypasses**: Exploits that trick the system into classifying a `NON_IDEMPOTENT_WRITE` tool as `PURE` or `READ_ONLY`, bypassing the Fail-Closed default.
- **Block-and-Gate Evasion**: Flaws that allow an interrupted non-idempotent tool to automatically retry without explicit human/policy resolution.
- **Backend Injection**: Any flaw in `drivers/durable-backend/dbos/` that allows arbitrary code execution outside of the locked DBOS/Restate step wrapper context.

### B. State & Durability Plane
- **Seal Tampering**: Vulnerabilities allowing a node payload to be mutated without breaking the RFC 8785 (JCS) + SHA256 identity hashing.
- **Cache Poisoning (`k8s-fluid` profile)**: Exploits where a stale or poisoned Fluid Dataset FUSE mount can trick the runtime into bypassing the direct S3/MinIO CAS hash-verification fallback.

### C. Data & Export Plane (`.tir` Packages)
- **Sensitive Data Leakage**: Flaws where nodes marked with the `SENSITIVE` effect class fail to be stripped or hashed properly during a `redacted` `.tir` package export.

## 4. Security Accountability for Contributors

As defined in our [Contributing Guidelines](CONTRIBUTING.md):
1. **AI Generation Liability**: If you use AI coding assistants (Antigravity IDE, Claude Code, ECC) to draft PRs, **you, the human contributor, are 100% accountable** for any security flaws they introduce. AI agents have zero built-in trust regarding security boundaries.
2. **Mandatory Security Reviews**: Any pull request that modifies files in `pkg/effects/` (tool safety mapping) or `pkg/resume/` (block-and-gate semantics) is automatically flagged for maximum scrutiny and requires sign-off from the **Security-Review Agent** and a human core maintainer.

Thank you for helping keep Trajectory IR safe and verifiable!
