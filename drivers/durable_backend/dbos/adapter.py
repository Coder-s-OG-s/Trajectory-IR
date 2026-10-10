import os
import sqlite3

from dbos import DBOS, DBOSConfig


def _sqlite_file(url: str) -> str | None:
    prefix = "sqlite:///"
    if not url.startswith(prefix) or ":memory:" in url:
        return None
    return url[len(prefix):]


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
    return DBOS.step()(fn)


# Tool calls MUST go through this wrapper -- never invoked raw inside a
# @durable_workflow-decorated function.
def durable_tool(fn):
    return DBOS.step()(fn)


def durable_workflow(fn):
    return DBOS.workflow()(fn)
