# Trajectory IR Infrastructure & Deployment Design

This document serves as the absolute blueprint for Trajectory IR. It defines both the **production architecture** (how the system runs) and the **developer infrastructure** (how we build, test, and ship this project day-to-day).

---

## Part 1: Production Infrastructure & Architecture

Trajectory IR operates as a semantic layer over existing execution and storage primitives. It is divided into three operational planes.

### 1.1 System Architecture & Component Interaction

```mermaid
flowchart TD
    subgraph Execution Plane
        A[Agent / LLM] -->|Proposes Plan| B(Trajectory IR Core)
        B -->|Wraps Tool Calls| C{Durable Backend Adapter}
    end

    subgraph State & Durability Plane
        C -->|Crash-safe execution, Retries| D[(DBOS / Restate)]
        B -->|Appends Nodes/Seals| E[(IR Metadata Log)]
        D -.->|Shares DB| E
    end

    subgraph Data & Caching Plane
        B -->|Writes Artifacts| F[(S3 / MinIO CAS)]
        B -->|Reads Artifacts| G{Cache Miss / Fallback}
        G -->|Direct Fetch| F
        G -->|Reads via Mount| H[(Fluid Dataset)]
        H -.->|Async Sync| F
    end
```

### 1.2 Storage Mechanics & Schemas

**IR Metadata Log (SQLite/Postgres)**
- `trajectories`: `trajectory_id` (PK), `tenant_id`, `status`.
- `nodes`: `node_id` (PK, computed via RFC 8785 JCS + SHA256), `trajectory_id`, `seq`, `kind`, `payload`.
- `seals`: `seal_id` (PK), `node_id` (FK), `content_hash`. *(Note: Contains the deterministic RFC 8785 JCS + SHA256 content integrity hash; package-level cryptographic digital signatures are explicitly deferred out-of-scope for Phase 1A).*

**Sharded CAS Object Store (S3/Filesystem)**
Artifacts are sharded by the first two hex characters of their SHA256 hash to prevent bucket listing degradation:
```text
s3://<bucket_name>/cas/<shard_prefix>/<full_hash>
# Example: s3://trajir/cas/e3/b0c4...
```

### 1.3 Deployment Profiles

| Profile | Target Environment | Execution | Database | Storage | Caching |
|---|---|---|---|---|---|
| `local` | Dev / Phase 1A | DBOS (Embedded) | SQLite | Local FS | None |
| `server-s3` | Single-Region API | DBOS / Restate | PostgreSQL | AWS S3 | None |
| `k8s-fluid` | Enterprise Fleets | K8s Deployments | PostgreSQL | AWS S3 | Fluid FUSE |

---

## Part 2: Developer Infrastructure (How We Build This Project)

To ensure high quality, deterministic builds, and rapid iteration, we enforce strict developer infrastructure protocols.

### 2.1 The Local Developer Environment

1. **Python Toolchain**:
   - Version: **Python 3.11+**
   - Package Manager: **Hatch** (via `pyproject.toml`) for deterministic dependency resolution.
   - Linting & Formatting: **Ruff** (replaces Flake8, Black, Isort).
   - Type Checking: **Mypy** (Strict mode enabled for all core IR packages).

2. **Core Dependencies**:
   - `rfc8785`: RFC 8785 JCS for stable payload hashing (do not use `canonicaljson`).
   - `dbos`: Embedded durable execution backend for Phase 1A.
   - `pytest` & `pytest-cov`: Unit tests now; conformance suite when R01/R02 land.

### 2.2 AI Agent Workflow (ECC Integration)

This repository is maintained by human owners collaborating with AI agents (specifically the Antigravity IDE and the Everything Claude Code [ECC] specialized subagent suite). As documented in `README.md` Section 15, we mandate the following developer and AI workflow:

1. **Planner Agent**: Must be invoked for any new architecture or module to draft an `implementation_plan.md` before coding.
2. **TDD-Guide Agent**: All modules in `pkg/` and `drivers/` are built test-first. Test coverage must exceed 80%.
3. **Security-Review Agent (Procedural Governance Gate)**: Must be invoked before merging any modifications to `pkg/effects/` (tool safety boundaries) and `pkg/resume/` (block-and-gate logic). Unlike automated CI checks, this is a mandatory procedural code review and human maintainer sign-off policy designed to ensure maximum scrutiny on sensitive boundary logic.

### 2.3 CI/CD Pipeline (GitHub Actions)

Workflow file: `.github/workflows/ci.yml`.

**What runs on every pull request today**

```mermaid
sequenceDiagram
    participant PR as Pull Request
    participant DCO as DCO job
    participant Q as Quality job

    PR->>DCO: Signed-off-by on each commit
    alt DCO fails
        DCO-->>PR: Block
    else DCO passes
        PR->>Q: install, Ruff, Mypy, import smoke, pytest
        alt Quality fails
            Q-->>PR: Block
        else Quality passes
            Q-->>PR: Allow merge from CI side
        end
    end
```

| Gate | Status | Notes |
|------|--------|--------|
| DCO (`Signed-off-by`) | **Hard gate now** | `dco-check` job on pull requests |
| Ruff + Mypy + import smoke + pytest | **Hard gate now** | Matrix on Python 3.11 and 3.12 |
| R01 / R02 conformance | **Planned hard gate** | Wire into CI when tests exist under `conformance/` |
| Branch protection on `main` | **Maintainer settings** | See `docs/maintainer-branch-protection.md` |
| Effects / resume review | **Procedural** | Human review for safety-sensitive paths |

Phase 1A is not complete until R01 and R02 pass (see master README and issue #2). That is a product milestone gate. Until those tests exist, CI does not claim they are already running.

### 2.4 Codebase Mapping

When building, code must be placed strictly according to this physical infrastructure layout:

* **`spec/`**: Design docs (including this file).
* **`pkg/trajectory_ir/runtime/`**: Core logic (Nodes, Trajectory logic, JCS hashing). No DBOS/backend code belongs here.
* **`pkg/trajectory_ir/effects/`**: Tool safety classes and MCP mappings.
* **`pkg/trajectory_ir/resume/`**: The block-and-gate protocol semantics.
* **`drivers/durable-backend/dbos/`**: The ONLY place where DBOS imports and workflow step wrappers exist.
* **`conformance/`**: The R01-R08 tests that prove the drivers work. 
* **`examples/kill-mid-deploy/`**: A runnable E2E harness demonstrating crash-safety in the real world.

> [!WARNING]
> **Boundary Violation Rule**: The `pkg/trajectory_ir/runtime/` module must **never** import `dbos`. All durable execution logic must remain completely isolated behind the interface in `drivers/durable-backend/`.
