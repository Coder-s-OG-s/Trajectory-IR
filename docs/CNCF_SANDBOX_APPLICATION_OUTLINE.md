# CNCF Sandbox application outline and readiness checklist

**Audience:** Trajectory IR maintainers preparing for CNCF Sandbox (next 2–3 months).  
**Not engineering work.** This is the application pack outline, form map, and
honesty checklist. Fill drafts here before filing on
[cncf/sandbox](https://github.com/cncf/sandbox).

| | |
|---|---|
| **Target maturity** | Sandbox (entry), not Incubation |
| **Apply when** | Checklist “must pass” items are green and narrative is stable |
| **Official form** | [Sandbox application issue form](https://github.com/cncf/sandbox/issues/new?template=application.yml) |
| **Process** | [cncf/sandbox README](https://github.com/cncf/sandbox/blob/main/README.md), [TOC lifecycle](https://github.com/cncf/toc/blob/main/process/README.md) |
| **Onboarding after vote** | [project-onboarding template](https://github.com/cncf/sandbox/blob/main/.github/ISSUE_TEMPLATE/project-onboarding.md) |

> **Caution (CNCF):** Do not claim the project is “donated” or “contributed” until
> the TOC votes **Approved** and the Contribution Agreement is signed.

---

## 1. One-sentence pitch (practice until automatic)

**Draft (edit until every maintainer agrees):**

> Trajectory IR is a portable, hash-verifiable intermediate representation for
> agent execution trajectories (typed nodes, sealed decisions, effect classes,
> thin/fat `.tir` packages) that runs **on top of** existing durable execution
> backends—not a replacement for Temporal, DBOS, or agent frameworks.

**What TOC should hear:**

| Message | Support |
|---------|---------|
| Cloud native fit | Agents on K8s/cloud; portable audit/export across runtimes |
| Novel gap | Runtime-independent, effect-safe unit of “what the agent did” |
| Non-overlap | Durable engines own crash/retry; we own semantics + portability |
| Spec-shaped | Conformance R01–R08, dual SDK (Go primary / Python reference) |

---

## 2. Honest readiness snapshot (as of v0.2.1 era)

Update this table monthly.

| Area | Status | Notes |
|------|--------|--------|
| Apache-2.0 project license | **Ready** | `LICENSE` |
| Public code, works | **Ready** | Go + Python, releases `v0.2.0` / `v0.2.1` |
| DCO on contributions | **Ready** | CI DCO job + CONTRIBUTING |
| Code of Conduct file | **Ready** | `CODE_OF_CONDUCT.md` (confirm README deep link) |
| Contributing guide | **Ready** | `CONTRIBUTING.md` |
| Security policy | **Check** | Ensure `SECURITY.md` exists and is linked |
| MAINTAINERS.md (Name / GitHub ID / **Company**) | **Gap** | Required for form; auto-close if missing |
| GOVERNANCE.md | **Gap** | Expected soon; strong for review |
| ROADMAP (public URL) | **Gap** | Form requires roadmap URL |
| ADOPTERS (optional) | **Gap** | Helps a lot if non-empty |
| Org diversity of maintainers | **Risk** | TOC considers employer diversity |
| Neutral GitHub org transfer-ready | **Plan** | Onboarding requires neutral org |
| OpenSSF Best Practices badge | **Start** | Not Sandbox-hard-required; expected for maturity |
| Scorecard / CI harden | **In progress** | Phase CI/CD harden |
| Landscape listing | **No** | After acceptance typically |
| Repo age 6+ months | **Verify** | Auto-close risk if younger than 6 months |
| Product decoupling story | **Draft needed** | Required form field |

---

## 3. Pre-submission checklist (CNCF auto-close risks)

Copied and adapted from the official form “Pre-Submission Checklist.”  
**Do not open the cncf/sandbox issue until “Critical” is all checked.**

### Critical (application closed without TOC review if failed)

- [ ] Project uses **Apache-2.0** (or allowlist license) **now**, not “after acceptance”
- [ ] **MAINTAINERS.md** (or `MAINTAINERS`) with table: **Name | GitHub ID | Company/Organization**
- [ ] Form links to that file via **GitHub `/blob/` path**, not contributors graph
- [ ] Primary repository is **≥ 6 months old** with active development
- [ ] Project is a **reusable tool/library**, not a reference architecture or company platform demo
- [ ] If splitting from a parent org/project: public parent maintainer consensus issue linked
- [ ] Dependency licenses are on the [CNCF third-party allowlist](https://github.com/cncf/foundation/blob/main/policies-guidance/allowed-third-party-license-policy.md) (or documented plan with FOSSA/Snyk)

### Strongly recommended before review

- [ ] README links CoC, Contributing, Security, license, quickstart
- [ ] Public **roadmap** URL (issues milestone or `docs/ROADMAP.md`)
- [ ] **SECURITY.md** with private reporting path
- [ ] **GOVERNANCE.md** (decision process, maintainers, emeritus)
- [ ] Clear **scope and non-goals** (durable engines, agent frameworks, memory products)
- [ ] **Similar projects** section ready (Temporal, Restate, DBOS, LangGraph, OTel, MCP)
- [ ] At least a thin **ADOPTERS.md** or “early interest” list (even design partners)
- [ ] Contribution Agreement signatories identified (legal can sign)
- [ ] Maintainers agree trademarks/accounts can transfer to LF if accepted
- [ ] TAG awareness (optional GTR / presentation notes linked)

### Common auto-closure mistakes (do not do these)

- Linking contributors graph instead of MAINTAINERS file  
- “Will add MAINTAINERS after acceptance”  
- BSL/GPL or “convert to Apache later”  
- Submitting a company reference platform as a project  
- Claiming “donated to CNCF” before vote + agreement  

---

## 4. Official form map (draft answers)

Fill the right column offline; paste into the GitHub form when ready.

### 4.1 Basic project information

| Form field | Draft content / instructions |
|------------|------------------------------|
| **Project summary** (one line) | Portable hash-verifiable intermediate representation for agent execution trajectories (seals, effects, `.tir`). |
| **Project description** (100–300 words) | See §5 draft paragraph. |
| **Reusable project checkbox** | Must check yes. Emphasize SDK + package format, not a single vendor stack. |

### 4.2 Project details

| Form field | Draft / repo pointer |
|------------|----------------------|
| **Org repo URL** | Today: org hosting repo, or `N/A` if single-repo. Prefer neutral org before or soon after apply. |
| **Project repo URL** | `https://github.com/Coder-s-OG-s/Trajectory-IR` (update if org moves) |
| **Additional repos** | List only in-scope repos (docs-only ok if separate). |
| **Website URL** | Website or primary repo URL if none. |
| **Roadmap** | **Must exist.** Create `docs/ROADMAP.md` or public milestone board URL. |
| **Roadmap context** | Phase done: 1A library, 1B Go primary, 1C harden. Next: CI/CD harden, adoption, interop demos. Explicit non-goals: SaaS control plane, Fluid productization, custom crash engines. |
| **Contributing guide** | `.../blob/main/CONTRIBUTING.md` |
| **Code of Conduct** | `.../blob/main/CODE_OF_CONDUCT.md` (and README pointer) |
| **Adopters** | Optional URL; better non-empty than silent. |
| **Maintainers file** | **Required** `.../blob/main/MAINTAINERS.md` with company column. |
| **Security policy** | `.../blob/main/SECURITY.md` |
| **Standard or specification?** | Yes, partially: normative IR / package semantics in root README (spec-v0.2-draft); Python + Go ports; conformance R01–R08. Not an ISO standard. |
| **Business product separation** | State honestly. If unrelated to a commercial product: *“This project is unrelated to any product or service.”* If related: describe upstream OSS vs product boundary (org, branding, governance). |

### 4.3 Cloud native context

| Form field | Draft talking points |
|------------|----------------------|
| **Why CNCF?** | Neutral home for a portable agent IR; alignment with cloud native AI / storage white paper themes; durable execution + K8s ecosystems already live in CNCF-adjacent space; need vendor-neutral format for audit and multi-runtime handoff. |
| **Benefit to landscape** | Adds **portable semantics** layer missing from engines (Temporal/etc.) and frameworks (checkpoints locked to one runtime). Enables export/import of agent runs (`.tir`) with effect-class safety. |
| **Cloud native fit** | Stateless libraries + portable packages; integrates with cloud-native durable backends; ops-friendly audit artifact; works in multi-service / multi-cluster agent deployments. |
| **Cloud native integration** | Complements Temporal (Go durable), DBOS (Python reference), optional Restate; maps MCP tool annotations into effect classes; potential OTel correlation later (trace IDs in nodes—roadmap, not claim as shipped). |
| **Cloud native overlap** | Overlap is **complementary**: engines provide durability; we do not reimplement crash detection/retry/leases. No claim to replace OTel as telemetry standard. |
| **Similar projects** | Temporal, Restate, DBOS (engines); LangGraph/CrewAI/ADK checkpoints (framework-local); MCP annotations (tool hints only); Mem0/Zep (memory quality, not trajectory IR). |
| **Landscape** | Not listed yet (expected answer until accepted). |
| **LFX Insights** | Not listed yet unless already enrolled. |

### 4.4 CNCF policies

| Form field | Answer |
|------------|--------|
| **Trademark and accounts** | Must check: donate trademarks/accounts if accepted. |
| **IP policy** | Must check: follow CNCF IP Policy. |
| **License exception** | `N/A - Project uses Apache 2.0 license` (if true). |
| **Domain Technical Review / TAG** | Optional but high value: link TAG presentation notes or Day-0 GTR answers. |

### 4.5 Contacts

| Form field | Prepare |
|------------|---------|
| **Application contact emails** | Maintainers who will answer TOC questions quickly. |
| **Contribution Agreement signatory** | Legal entity table (name, address, type, signatory, email) **or** individual table. Align with IP ownership of the codebase. |

### 4.6 Additional information

| Form field | Suggestions |
|------------|-------------|
| **CNCF contacts** | TOC/TAG people who already know the project (if any). |
| **Additional information** | Link releases, conformance, dual SDK, CI, white paper §, Phase 1B/1C status docs. |

---

## 5. Draft “Project description” (100–300 words)

*Edit before paste.*

Trajectory IR defines a **typed, append-only intermediate representation for agent
runs**. A run is a trajectory of nodes (context, decision, tool call/result,
commit, abort, artifacts). Before world-changing tools execute, the model’s plan
is **sealed**; resume replays the seal rather than silently re-inferring. Tools
carry **effect classes** (including fail-closed mapping from MCP annotations),
and **block-and-gate** prevents unsafe retry of non-idempotent side effects.

Trajectory IR is **not** a durable execution engine. Crash detection, leases, and
at-most-once execution are delegated to backends such as Temporal (Go) or DBOS
(Python). The project owns the agent-specific semantics and a **portable,
hash-verifiable package format** (`.tir`, thin and fat) so a trajectory can leave
the runtime that produced it—for audit, handoff, or another implementer.

The cloud native gap is the lack of a runtime-independent unit of “what the agent
actually did” across frameworks and engines. Trajectory IR targets that unit with
a dual SDK (Go primary, Python reference), conformance tests R01–R08, and local
integration paths for Postgres NodeLog and S3-compatible CAS.

---

## 6. Draft “Why CNCF?” bullets

- **Neutrality:** Portable IR only works if it is not owned by one agent framework vendor.  
- **Ecosystem:** Cloud native agent workloads already sit on K8s, service mesh, and durable workflows; IR belongs next to those building blocks.  
- **Interoperability mission:** CNCF is the natural home for cross-project contracts (cf. how OTel standardized telemetry).  
- **What we need from CNCF:** Visibility, governance scaffolding, landscape placement, community channels—not a commercial product home.  
- **What we will not ask for:** Replacing Temporal/OTel or building a hosted multi-tenant SaaS under CNCF.

---

## 7. Differentiation table (for “overlap / similar projects”)

| System | Solves | Trajectory IR does **not** | Trajectory IR adds |
|--------|--------|----------------------------|--------------------|
| Temporal / Restate / DBOS | Durability, retry, leases | Reimplement crash engines | Agent seal semantics + portable export |
| LangGraph / CrewAI / ADK | Framework checkpointing | Be “the” agent framework | Format that can leave the framework |
| MCP annotations | Tool safety hints | Rival annotation standard | Map into effect classes + resume matrix |
| OpenTelemetry | Traces/metrics/logs | Replace OTel | Optional correlation of IR nodes later |
| Mem0 / Zep | Long-term memory quality | Compete on recall | Optional LTM node shapes only |

---

## 8. Documents to create before applying (content plan)

No need to invent product features—create **process artifacts** TOC expects.

| Document | Purpose | Owner |
|----------|---------|--------|
| `MAINTAINERS.md` | Form-critical; company column | Maintainers |
| `GOVERNANCE.md` | How decisions and maintainers work | Maintainers |
| `docs/ROADMAP.md` | Public roadmap URL for form | Maintainers |
| `ADOPTERS.md` | Early users / design partners | Maintainers |
| `SECURITY.md` | If missing or thin, complete | Security owner |
| `docs/CNCF_SANDBOX_APPLICATION_OUTLINE.md` | This file | Maintainers |
| Sandbox issue draft (offline copy) | Paste into form | Applicant |

Optional but strong:

| Document | Purpose |
|----------|---------|
| `docs/SCOPE_AND_NON_GOALS.md` or README section | Freeze scope |
| Architecture diagram (one page) | TAG presentation |
| Interop note: export `.tir` across two runtimes | “Benefit to landscape” proof |

---

## 9. 90-day prep calendar (no engineering feature work required)

### Month 1 — Eligibility and packaging

- [ ] Confirm repo age ≥ 6 months  
- [ ] Write **MAINTAINERS.md** (real employers)  
- [ ] Write **GOVERNANCE.md** (even v0.1)  
- [ ] Publish **docs/ROADMAP.md**  
- [ ] Tighten **SECURITY.md** + vulnerability contact  
- [ ] License scan dependencies vs CNCF allowlist  
- [ ] Agree product separation paragraph  
- [ ] Start OpenSSF Best Practices badge (do not finish in one week)  
- [ ] Finish CI/Scorecard wave (in flight)  

### Month 2 — Narrative and TAG touch

- [ ] Freeze one-line + 200-word description  
- [ ] Complete similar/overlap tables with maintainers  
- [ ] Optional: present to a relevant TAG / Project Reviews voluntary GTR Day 0  
- [ ] Collect 1–2 adopter or design-partner quotes  
- [ ] Identify Contribution Agreement signatory  
- [ ] Decide neutral GitHub org naming (for post-accept transfer)  

### Month 3 — Apply only if ready

- [ ] Re-run critical checklist  
- [ ] Open [Sandbox application](https://github.com/cncf/sandbox/issues/new?template=application.yml)  
- [ ] Respond quickly to TOC comments (`Need-Info` → `Completed info, project is Returning`)  
- [ ] **Do not** market as CNCF project until Approved + agreement signed  

---

## 10. After acceptance (preview only)

Staff open onboarding (~1 month target). You will need to:

1. Sign Contribution Agreement; transfer trademarks/domains/accounts as applicable  
2. Move to **neutral org** then CNCF GHE  
3. DCO, CoC in README, governance/security/OpenSSF badge progress  
4. FOSSA or Snyk for license policy  
5. Artwork, Slack, maintainers list, LFX profiles  

Full list: [project-onboarding.md](https://github.com/cncf/sandbox/blob/main/.github/ISSUE_TEMPLATE/project-onboarding.md).

---

## 11. What not to optimize for Sandbox

| Avoid as primary push | Why |
|----------------------|-----|
| Large new product surface (SaaS, Fluid, signatures) | Dilutes “portable IR” story; Future only |
| Claiming to replace Temporal/LangGraph | Overlap trap |
| Stars campaigns without adopters | Weak due diligence |
| Applying with single-company maintainers and no governance file | High postpone risk |
| “We will open source governance after accept” | Same as license promises |

---

## 12. Suggested internal RACI

| Workstream | Responsible | Accountable |
|------------|-------------|-------------|
| Application text | Tech lead + PM voice | Maintainer group |
| Legal / Contribution Agreement | Legal / signatory | Org executive |
| MAINTAINERS / governance | All maintainers | Lead maintainer |
| Security / OpenSSF | Security-minded maintainer | Lead maintainer |
| TAG presentation | Presenter + backup | Lead maintainer |
| Community/adopters | Outreach | Lead maintainer |

---

## 13. Decision gate: “Apply now?” vs “Wait”

**Apply when all are true:**

1. Critical checklist complete  
2. MAINTAINERS has ≥1 real company row per person and looks honest  
3. Roadmap and non-goals published  
4. Product separation sentence agreed  
5. Signatory ready within days of form submission  
6. Pitch is stable for 30 days (no thrash)  

**Wait if any are true:**

1. License or dependency license mess  
2. No MAINTAINERS file  
3. Pure single-vendor product with no OSS boundary story  
4. Repo narrative still “startup product” not “portable contract”  
5. Nobody available to answer TOC for 2–4 weeks after filing  

---

## 14. References

- [How to submit a project](https://contribute.cncf.io/projects/submit-project/)  
- [Sandbox applications repo](https://github.com/cncf/sandbox)  
- [Sandbox application form](https://github.com/cncf/sandbox/issues/new?template=application.yml)  
- [TOC project lifecycle](https://github.com/cncf/toc/blob/main/process/README.md)  
- [CNCF IP Policy (Charter §11)](https://github.com/cncf/foundation/blob/main/charter.md#11-ip-policy)  
- [Third-party license allowlist](https://github.com/cncf/foundation/blob/main/policies-guidance/allowed-third-party-license-policy.md)  
- [Sandbox Review Guide (optional GTR)](https://github.com/cncf/toc/blob/main/toc_subprojects/project-reviews-subproject/sandbox-review-guide.md)  
- [General Technical Review questions](https://github.com/cncf/toc/blob/main/toc_subprojects/project-reviews-subproject/general-technical-questions.md)  
- Internal: [CI_HARDENING.md](CI_HARDENING.md), [PHASE_1C_STATUS.md](PHASE_1C_STATUS.md), root README §3–§5  

---

## 15. Next actions for maintainers (this week)

1. Create **MAINTAINERS.md** with Name / GitHub ID / Company.  
2. Draft **GOVERNANCE.md** and **docs/ROADMAP.md**.  
3. Assign owners for OpenSSF badge and SECURITY.md review.  
4. Hold a 60-minute meeting to freeze the one-line pitch and product-separation sentence.  
5. Do **not** file cncf/sandbox until §3 Critical is fully checked.  

When those five are done, paste §4 into the official form and submit.
