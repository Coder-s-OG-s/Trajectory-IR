# kill-mid-deploy demo

Demonstrates the milestone's core claim: a crash mid-tool-execution never silently re-runs the model or the side effect on resume.

## Run it

First, ensure the repository is installed locally:
```bash
pip install -e .
```

```bash
python examples/kill-mid-deploy/run_demo.py
# in another terminal, once you see "TOOL_CALL: deploy_server started":
kill -9 <pid>
python examples/kill-mid-deploy/run_demo.py --resume
```

Expected final output: `Resumed. deploy_server executed exactly once.`

## Recording

Once the demo runs reliably (no timing flakiness), record it:

```bash
asciinema rec demo.cast
```

A recorded run is the actual launch asset for this milestone — more persuasive to anyone evaluating the project cold than the spec document alone.
