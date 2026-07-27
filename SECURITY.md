# Security Policy

Security is a primary concern for Trajectory IR. Because this framework acts as the underlying execution and state-management layer for autonomous AI agents, vulnerabilities here could lead to unauthorized tool execution, state manipulation, or the leakage of sensitive data (like PII or secrets).

## Supported Versions

Currently, Trajectory IR is in its **Phase 1A / v0.1.x** development cycle.

| Version | Supported          |
| ------- | ------------------ |
| v0.1.x  | :white_check_mark: |
| < v0.1  | :x:                |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability within Trajectory IR, please send an e-mail to the core maintainers team. *(Note: Maintainer email to be added).*

We will acknowledge receipt of your vulnerability report within 48 hours and strive to send you regular updates about our progress. If you have not received a reply within 48 hours, please reach out to the project owner directly.

## Scope of Security Concerns

We are particularly interested in reports concerning:
- **Tool Safety Bypasses**: Any exploit that allows an agent to bypass the Fail-Closed default (`NON_IDEMPOTENT_WRITE`) or the Block-and-Gate execution policy.
- **Seal Tampering**: Any vulnerability allowing a node or step seal to be silently modified without breaking the SHA256 / JCS hashing verification.
- **Sensitive Data Leakage**: Flaws where nodes marked with the `SENSITIVE` effect class fail to be redacted properly during `.tir` package exports.
- **Execution Backend Injection**: Any flaw in the `durable-backend-adapter` that allows execution of arbitrary code outside of the defined Trajectory context.

Thank you for helping keep Trajectory IR safe!
