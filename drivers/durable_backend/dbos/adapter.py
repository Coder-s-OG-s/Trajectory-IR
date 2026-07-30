import os

from dbos import DBOS, DBOSConfig


def init_backend(app_name: str = "trajectory-ir-local") -> None:
    config: DBOSConfig = {
        "name": app_name,
        "system_database_url": os.environ.get(
            "DBOS_SYSTEM_DATABASE_URL", f"sqlite:///{app_name}.sqlite"
        ),
    }
    DBOS(config=config)
    DBOS.launch()


# Model inference MUST be wrapped identically to tool calls (spec §1 fix):
# without this, DBOS replays the whole workflow body on crash-resume and the
# model is silently re-invoked even though its output is discarded once
# execution reaches the memoized DECISION step. Do not simplify this away.
def durable_infer(fn):
    return DBOS.step()(fn)


# Tool calls MUST go through this wrapper -- never invoked raw inside a
# @durable_workflow-decorated function.
def durable_tool(fn):
    return DBOS.step()(fn)


def durable_workflow(fn):
    return DBOS.workflow()(fn)
