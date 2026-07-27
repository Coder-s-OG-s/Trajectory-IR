# Trajectory IR Infrastructure & Deployment Design

This document details the step-by-step infrastructure design for the Trajectory IR project. It synthesizes the storage, execution, and deployment constraints specified in the master `README.md` into actionable architecture tiers.

## 1. Core Architecture Layers

Trajectory IR is designed as a semantic and portability layer over existing execution and storage primitives. It deliberately does not reinvent durable execution or object storage.

| Concern | Technology / Component | Responsibility |
|---|---|---|
| **Durable Execution (Backend)** | **DBOS** (Phase 1A Default) <br> *Restate (Future)* | Owns crash detection, retry policies, deterministic replay, and lease/heartbeat coordination. Trajectory IR wraps tools as durable steps using this backend. |
| **IR Metadata & Node Log** | **SQLite** (Local) <br> **PostgreSQL** (Server) | Stores trajectory node sequences, seals, and system states. Also used by DBOS for its own workflow state. |
| **Content Addressed Storage (CAS)** | **Local Filesystem** <br> **MinIO / AWS S3** | Durable storage of artifact bytes, strictly identified by SHA256 hashes (`cas/<first-2-hex>/<remaining>`). |
| **Caching / Data Locality** | **Fluid** (CNCF) | Optional read-path accelerator for large datasets on Kubernetes. |

> [!CAUTION]
> **Strict Invariant**: Trajectory IR's correctness must *never* depend on a cache (Fluid or otherwise). Cache misses are expected and must seamlessly fallback to verifying hashes directly against the durable CAS store.

---

## 2. Deployment Profiles (Step-by-Step Evolution)

The infrastructure scales across three distinct profiles. The API surface remains identical across all profiles.

### Step 1: `local` Profile (Phase 1A Target)
Designed for rapid development and the initial open-source release with zero operational overhead.
- **Execution Backend**: DBOS (running embedded within the Python SDK).
- **Database**: **SQLite** (Single file handles both DBOS workflow state and Trajectory IR node logs).
- **Storage**: Local filesystem directory acting as the CAS object store.
- **Containerization**: None required. Runs natively via Python for frictionless contributor onboarding.

### Step 2: `server-s3` Profile (Single-Region API)
Designed for standalone API deployments without heavy Kubernetes orchestration.
- **Execution Backend**: DBOS (embedded) or Restate (standalone server).
- **Database**: **PostgreSQL** for high-concurrency node logs and durable step states.
- **Storage**: **Amazon S3** (production) or **MinIO** (CI/CD and local server testing) using standard S3 SDKs.
- **Containerization**: Packaged via `Dockerfile.server`.

### Step 3: `k8s-fluid` Profile (Multi-Pod Agent Fleets)
Designed for enterprise scale where multiple agents process massive shared datasets (e.g., repository snapshots, large context windows).
- **Execution Backend**: PostgreSQL-backed DBOS / Restate on Kubernetes.
- **Storage**: Amazon S3 / MinIO.
- **Caching**: **Fluid Dataset/Runtime** deployed via Helm. Fluid mounts the read-heavy corpora into pods for fast localized reads.
- **Multi-Tenancy**: Single shared Fluid Dataset with *path-based isolation*, rather than one dataset per tenant.
- **Containerization**: Deployed via `charts/trajectory-ir/` Helm chart and `Dockerfile.k8s-fluid`.

---

## 3. Technology Stack Requirements

> [!IMPORTANT]
> To preserve consistency, the project explicitly mandates the following stack. Deviations require an approved issue.

- **Primary Language**: Python (Phase 1A SDK, embedded runtime, and test harness).
- **Future Control Plane Language**: Go (reserved for Kubernetes-native components).
- **Data Hashing**: RFC 8785 JSON Canonicalization Scheme (JCS).
- **CI/CD Quality Gates**: 
  - GitHub Actions.
  - Required DCO sign-offs (`Signed-off-by:`).
  - Passing Conformance Tests (`R01` & `R02` are hard gates).

---

## 4. CAS Storage Layout Specification

Artifacts must not be dumped into a flat directory, which creates metadata-listing pressure at scale. They must be sharded by the first two characters of their SHA256 hash.

**Required Layout:**
```text
s3://<bucket_name>/cas/<first-2-hex-chars>/<remaining-hex-chars>
# Example: s3://trajir/cas/ab/cdef0123...
```
This is required starting from Phase 1A to ensure smooth transition to Fluid and S3 without data migration overhead.
