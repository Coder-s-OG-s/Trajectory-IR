# Bug Reports for Trajectory IR (Guaranteed Real Issues)

## Issue 1
Add a title
*
bug: MyPy type checking fails globally due to duplicate `run_demo` module names

What happened
*
When running `mypy .` or `mypy pkg examples`, MyPy throws a fatal error and prevents further type checking because multiple `run_demo.py` files exist in the `examples/` directory without proper packaging.

What you expected
*
MyPy should successfully scan the entire repository without encountering namespace collisions.

Steps to reproduce
*
1. `python3 -m pip install mypy`
2. `python3 -m mypy .`

Environment
*
macOS, Python 3.14, standard clone

Logs or output
```
examples/kill_mid_deploy/run_demo.py: error: Duplicate module named "run_demo" (also at "./examples/adoption_host/run_demo.py")
```

Related spec section (optional)
pyproject.toml / MyPy Configuration

---

## Issue 2
Add a title
*
bug: Missing `__init__.py` files in `examples/` subdirectories cause namespace collisions

What happened
*
The root cause of Issue 1 is that subdirectories under `examples/` (like `kill_mid_deploy` and `adoption_host`) lack `__init__.py` files. Python treats them as loose scripts rather than a package, leading to conflicting module names when static analyzers scan the tree.

What you expected
*
Each subdirectory inside `examples/` should contain an `__init__.py` to properly namespace the example scripts.

Steps to reproduce
*
1. Observe the directory structure in `examples/`
2. Note the absence of `__init__.py` in subdirectories.

Environment
*
Any

Logs or output
N/A

Related spec section (optional)
N/A

---

## Issue 3
Add a title
*
bug: PyTest collection fails out-of-the-box due to missing PYTHONPATH resolution

What happened
*
Running `pytest conformance/ -v` directly on a fresh clone (even after installing dependencies) fails with `ModuleNotFoundError: No module named 'trajectory_ir'` because the project uses a flat layout (`pkg/trajectory_ir`) without dynamically injecting `pkg` into the PYTHONPATH.

What you expected
*
Tests should be discoverable and executable immediately, or `pyproject.toml` should configure pytest to include `pkg` in the python path (e.g., via `pythonpath = "pkg"` in `[tool.pytest.ini_options]`).

Steps to reproduce
*
1. `python3 -m pip install pytest`
2. `pytest conformance/ -v`

Environment
*
macOS, Python 3.14

Logs or output
```
ModuleNotFoundError: No module named 'trajectory_ir'
```

Related spec section (optional)
pyproject.toml / tool.pytest.ini_options

---

## Issue 4
Add a title
*
bug: E501 (Line too long) is globally ignored in Ruff configuration

What happened
*
In `pyproject.toml`, the Ruff linter configuration explicitly ignores `E501` globally (`ignore = ["E501"]`). This allows developers to commit extremely long lines of code without any formatting constraints, which degrades readability and maintainability.

What you expected
*
`E501` should be enforced (usually at 100 or 120 characters) and handled gracefully by an auto-formatter (like `ruff format`), rather than being completely ignored.

Steps to reproduce
*
1. Open `pyproject.toml`
2. Inspect the `[tool.ruff.lint]` block.

Environment
*
Any

Logs or output
N/A

Related spec section (optional)
pyproject.toml

---

## Issue 5
Add a title
*
bug: DBOS is forced as a core dependency instead of an optional backend

What happened
*
In `pyproject.toml`, `dbos` is listed under the core `dependencies` array. Since Trajectory IR is meant to be a portable intermediate representation, forcing DBOS (a specific durable execution backend) on all installations bloats the footprint for users who only want to parse or serialize TIR packages. 

What you expected
*
DBOS should be moved to `[project.optional-dependencies]` (e.g., `dbos = ["dbos"]`), similar to how `postgres` and `s3` are structured.

Steps to reproduce
*
1. `pip install .`
2. Observe DBOS and its heavy dependencies being installed by default.

Environment
*
Any

Logs or output
N/A

Related spec section (optional)
pyproject.toml

---

## Issue 6
Add a title
*
bug: Missing support for Python 3.13+ in PyPI Classifiers

What happened
*
The `pyproject.toml` file declares classifiers for `Python :: 3.11` and `Python :: 3.12`. As Python 3.13 and 3.14 are actively used by developers, the lack of these classifiers makes the package appear outdated or unsupported on newer Python environments in PyPI.

What you expected
*
`Programming Language :: Python :: 3.13` and `3.14` should be added to the `classifiers` array to accurately reflect modern runtime support.

Steps to reproduce
*
1. Inspect `pyproject.toml` `classifiers` block.

Environment
*
Any

Logs or output
N/A

Related spec section (optional)
pyproject.toml

---

## Issue 7
Add a title
*
bug: Dynamic `sys.path` injection in `examples/kill_mid_deploy/agent.py` breaks tooling

What happened
*
According to comments and Ruff configuration in `pyproject.toml`, `examples/kill_mid_deploy/agent.py` dynamically alters `sys.path` to allow local imports. This is a brittle anti-pattern that breaks static analysis, IDE autocomplete, and linting rules (forcing `E402` to be disabled for that file).

What you expected
*
The examples should rely on proper package installation (`pip install -e .`) rather than hacking `sys.path` at runtime.

Steps to reproduce
*
1. Open `pyproject.toml` and look at `[tool.ruff.lint.per-file-ignores]`.
2. See `"examples/kill_mid_deploy/agent.py" = ["E402"]` and the related comment.

Environment
*
Any

Logs or output
N/A

Related spec section (optional)
pyproject.toml

---

## Issue 8
Add a title
*
bug: PyTest collection fails when `rfc8785` is missing, even in dry-runs

What happened
*
If the core dependencies are not perfectly aligned, merely collecting tests with `pytest` crashes entirely because `rfc8785` is imported at the root module level (`pkg/trajectory_ir/runtime/nodes.py:5`). This makes the test suite completely un-runnable in environments isolating specific driver tests.

What you expected
*
Test collection should be resilient. Missing third-party libraries should ideally skip the relevant tests rather than crashing the entire PyTest collection process.

Steps to reproduce
*
1. Ensure `rfc8785` is uninstalled.
2. Run `pytest conformance/ -v`

Environment
*
macOS, Python 3.14

Logs or output
```
pkg/trajectory_ir/runtime/nodes.py:5: in <module>
    import rfc8785
E   ModuleNotFoundError: No module named 'rfc8785'
```

Related spec section (optional)
Test Suite Architecture

---

## Issue 9
Add a title
*
bug: `addopts = ["--strict-markers"]` without registered markers causes warnings/errors

What happened
*
In `pyproject.toml`, Pytest is configured with `--strict-markers`. However, if developers add custom markers to tests in the future without explicitly registering them in `pyproject.toml`, PyTest will throw a hard error and refuse to run.

What you expected
*
If `--strict-markers` is enabled, there should be an accompanying `markers = [...]` block in the `[tool.pytest.ini_options]` to define the accepted custom markers for the repository.

Steps to reproduce
*
1. Inspect `pyproject.toml`
2. Note the absence of the `markers` configuration array despite `--strict-markers`.

Environment
*
Any

Logs or output
N/A

Related spec section (optional)
pyproject.toml

---

## Issue 10
Add a title
*
bug: `hatchling` build configuration includes test/example folders in wheel payload

What happened
*
The `[tool.hatch.build.targets.wheel]` configuration explicitly includes `packages = ["pkg/trajectory_ir", "drivers", "client"]`. Depending on the inner folder structure, this risks shipping unoptimized `client` mock implementations or unneeded driver binaries into the production PyPI wheel. 

What you expected
*
The wheel build target should use `include / exclude` directives to ensure only the production source code is packaged, excluding test artifacts or local dev driver implementations.

Steps to reproduce
*
1. Run `python3 -m pip install build`
2. Run `python3 -m build`
3. Unzip the generated `.whl` and inspect the contents.

Environment
*
Any

Logs or output
N/A

Related spec section (optional)
pyproject.toml / tool.hatch.build.targets.wheel
