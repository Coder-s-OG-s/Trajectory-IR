# Compatibility

**Status: proposal.** Section 1 describes what is true on `main` today.
Section 2 is a policy maintainers have not agreed yet. Nothing in section 2
is a promise until this note changes to say it is adopted.

Trajectory IR is pre-1.0. Releases are `0.x`, the spec is `spec-v0.2-draft`,
and breaking changes have shipped in minor releases.

## 1. What is true today

### Package format

Every `.tir` names its format twice:

- `manifest.json` has `"package_format": "0.1"`
- `COMPAT.json` has `"package_format": "0.1"`, `"min_runtime": "0.1.0"`, and
  the node id and payload hash schemes

`0.1` has been the value since the first release. It has never been bumped.

Since spec 1.12 (§9, [#566](https://github.com/Coder-s-OG-s/Trajectory-IR/pull/566))
both readers check `package_format` before reading any node. It must be in
`manifest.json` and `COMPAT.json`, the two must match, it must be
`MAJOR.MINOR`, and MAJOR must be `0`. A newer minor such as `0.9` still loads.
Anything else fails with `tir.ErrFormat` in Go, `TirFormatError` in Python, and
`UNSUPPORTED_FORMAT` from `trajir verify`. Every package written by 0.1.0
through 0.4.0 passes this check.

`min_runtime`, when it is present, is checked against reader version `0.4.0`
(spec 1.13, [#563](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/563)).
It must be `MAJOR.MINOR.PATCH`. A higher value fails with `tir.ErrRuntime`
in Go and `TirRuntimeError` in Python. `trajir verify` reports
`RUNTIME_TOO_OLD`. The error names both versions. Writers still store
`0.1.0`, which is below `0.4.0`, so current packages load. A package that
omits the field still loads.

### Changes that broke earlier data or code

All of these shipped with `package_format` still `0.1`.

| Release | Change | Effect on existing users |
|---|---|---|
| 0.3.0 | Idempotency keys became a length-prefixed sha256; the colon form was withdrawn | A step in flight across the upgrade does not resume onto the new key. No translator |
| 0.3.0 | Python `client` and `drivers` moved under `trajectory_ir` | Host imports must change |
| 0.3.0 | Durable memo keys are scoped by tenant | A step in flight across the upgrade calls the model again. Gated writes still block |
| 0.4.0 | Go `Trajectory.Backend` removed | Code that called it no longer compiles. Create the backend and pass `client.Options.Backend`. `Trajectory.Log` is unchanged |
| 0.4.0 | Go rejects whole floats above `2^53-1` stored as integers | An existing `.tir` or log row with such a value now fails verification |

Each one is in `CHANGELOG.md`. Only the import move is marked
**BREAKING** there.

### Signatures

`trajir-pkg-sig-v1` is normative in the spec and has cross-language
conformance (R09 to R11). Packages signed by one SDK verify in the other.

## 2. Proposed policy

### Package format

1. `package_format` is `MAJOR.MINOR`.
   - **Minor:** additive only. New optional members or fields that an older
     reader can ignore and still verify correctly.
   - **Major:** anything else. A change to the node id or payload hash
     scheme, a new required member, or a change in meaning of an existing
     field.
2. A reader rejects a major version it does not know, and a `min_runtime`
   newer than itself, with an error that names both versions. Both checks
   are in the code. Go and Python carry runtime version `0.4.0`, the same
   value as `pyproject.toml`, and that constant has to move when the
   package version moves. Agreeing the rest of this list is still separate.
3. A `.tir` written by any release verifies with every later release that
   reads the same major format.
4. One exception to 3: a fix to hashing or canonicalisation that makes
   previously wrong packages fail. Those go under a `### Format` heading in
   `CHANGELOG.md` and say how to find affected files. The 0.4.0 integer
   fix is this case.

### SDK APIs (Go and Python)

5. Before 1.0, a minor release (`0.3` to `0.4`) may break the API. Each break
   is marked **BREAKING** in `CHANGELOG.md` with the before and after code.
6. A patch release (`0.3.0` to `0.3.1`) does not break the API or the
   package format.
7. Where it costs little, keep the old name working for one minor release
   and log a deprecation warning.

### Idempotency keys, seals and memo keys

8. Changing how any of these are derived is a breaking change even when the
   API is unchanged. The changelog entry says what happens to a step that is
   in flight during the upgrade, as the 0.3.0 entries do.

### At 1.0

9. Not decided. At minimum the package format major should be frozen, and
   the API rules above should move to normal semver (breaks only in a major
   release).

## Open questions for maintainers

- Bump `package_format` to `0.2` for the unreleased integer fix, or treat it
  as rule 4?
- Should `trajir verify` warn when a package is older than the reader's
  format, even when it still verifies?
- How long do we keep conformance fixtures from old formats in
  `testdata/`?
