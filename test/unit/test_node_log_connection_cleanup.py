"""NodeLog must release its SQLite connection when garbage collected."""

from __future__ import annotations

import gc
import sqlite3

import pytest

from trajectory_ir.runtime.log import NodeLog


def test_del_closes_connection_when_log_goes_out_of_scope(tmp_path):
    db_path = str(tmp_path / "cleanup.sqlite")
    log = NodeLog(db_path)
    log.append("DECISION", 1, {"plan": {}}, "t1", "demo", seq=1)
    conn = log._conn

    del log
    gc.collect()

    with pytest.raises(sqlite3.ProgrammingError):
        conn.execute("SELECT 1")


def test_del_after_explicit_close_does_not_raise(tmp_path):
    db_path = str(tmp_path / "cleanup_explicit.sqlite")
    log = NodeLog(db_path)
    log.close()

    del log
    gc.collect()
