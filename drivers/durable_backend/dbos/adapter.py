import os
import sqlite3
from functools import wraps

from dbos import DBOS, DBOSConfig

from trajectory_ir.runtime.nodes import UnsafePlan


def _sqlite_file(url: str) -> str | None:
    prefix = "sqlite:///"
    if not url.startswith(prefix) or ":memory:" in url:
        return None
    return url[len(prefix) :]


def init_backend(
    db_path: str = "trajectory.sqlite",
    app_name: str = "trajectory-ir-local",
) -> None:
    url = os.environ.get("DBOS_SYSTEM_DATABASE_URL", f"sqlite:///{db_path}")
    sqlite_file = _sqlite_file(url)
    if sqlite_file:
        conn = sqlite3.connect(sqlite_file)
        try:
            conn.execute("PRAGMA journal_mode=WAL")
        finally:
            conn.close()
    config: DBOSConfig = {
        "name": app_name,
        "system_database_url": url,
    }
    DBOS(config=config)
    DBOS.launch()


# Model inference MUST be wrapped identically to tool calls (spec §1 fix):
# without this, DBOS replays the whole workflow body on crash-resume and the
# model is silently re-invoked even though its output is discarded once
# execution reaches the memoized DECISION step. Do not simplify this away.
def durable_infer(fn):
    @wraps(fn)
    def checked(*args, **kwargs):
        try:
            return fn(*args, **kwargs)
        except UnsafePlan as exc:
            if exc.workflow_id is None:
                exc.workflow_id = DBOS.workflow_id
            raise

    return DBOS.step()(checked)


# Tool calls MUST go through this wrapper -- never invoked raw inside a
# @durable_workflow-decorated function.
def durable_tool(fn):
    return DBOS.step()(fn)


def durable_workflow(fn):
    wrapped = DBOS.workflow()(fn)

    @wraps(fn)
    def outer(*args, **kwargs):
        try:
            return wrapped(*args, **kwargs)
        except UnsafePlan as exc:
            workflow_id = exc.workflow_id
            if workflow_id:
                DBOS.delete_workflow(workflow_id)
            raise

    return outer
