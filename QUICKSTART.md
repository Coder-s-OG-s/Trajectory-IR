# Quickstart: Trajectory IR

Welcome to Trajectory IR! This guide will get you up and running with the `local` deployment profile in under 5 minutes. 

Trajectory IR acts as a semantic layer for AI agents, wrapping your agent's execution history in a portable, crash-safe format using [DBOS](https://docs.dbos.dev/).

## Prerequisites
- Python 3.11+
- `pip` or [Hatch](https://hatch.pypa.io/) (recommended)

---

## 1. Installation

Install the Trajectory IR SDK and the DBOS durable execution backend:

```bash
pip install trajectory-ir dbos-transact
```

## 2. Initialize the Local Environment

Trajectory IR requires a relational store for its metadata (Node tracking, Seals) and a Content Addressed Storage (CAS) layer for artifacts.

In the `local` profile, we use SQLite and the local filesystem:

```bash
# Initialize the local SQLite DB and sharded CAS directory
python -m trajectory_ir init --profile local
```
This command creates `~/.trajectory-ir/local.db` and `~/.trajectory-ir/cas/`.

## 3. Your First Durable Agent

Create a file called `agent.py`. In this example, we wrap a standard LLM function call inside Trajectory IR's durable execution context. If the script crashes mid-execution, running it again will safely resume from the exact same state without duplicating side-effects!

```python
from trajectory_ir.runtime import Trajectory
from trajectory_ir.effects import EffectClass
from dbos import DBOS

# 1. Initialize the durable backend (DBOS)
DBOS.launch()

# 2. Define a Tool with strict Effect Classification
@Trajectory.tool(effect_class=EffectClass.NON_IDEMPOTENT_WRITE)
def deploy_server(server_name: str):
    print(f"Deploying {server_name}...")
    return f"Success: {server_name} is live."

# 3. Create an Agent Workflow
@DBOS.workflow()
def run_agent():
    # Start a new semantic Trajectory
    traj = Trajectory.start(tenant_id="demo-user")
    
    # Execute the tool (Trajectory IR wraps this in a DBOS durable step)
    result = deploy_server("prod-web-01")
    
    # Append the result to the Trajectory Log
    traj.append_observation(result)
    
    # Export the trajectory as a portable .tir package
    tir_package = traj.export(mode="thin")
    print(f"Exported Trajectory IR: {tir_package}")

if __name__ == "__main__":
    run_agent()
```

## 4. Run and Verify

Execute your agent:
```bash
python agent.py
```

Because of the **Block-and-Gate** policy, if your agent crashes inside `deploy_server`, the next time you run `python agent.py`, it will recognize the interrupted `NON_IDEMPOTENT_WRITE` and halt execution, requesting human intervention.

## What's Next?
- Read the [Infrastructure Design](infrastructure.md) to learn how to scale this to `server-s3` or `k8s-fluid`.
- Read the [Contributing Guide](CONTRIBUTING.md) if you want to help build the project!
