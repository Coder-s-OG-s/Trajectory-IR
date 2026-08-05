"""PostgreSQL IR node log driver (server profiles).

The SQLite :class:`~trajectory_ir.runtime.log.NodeLog` remains the Phase 1A
default for the ``local`` profile. This package provides the same public
operations against PostgreSQL so multiple processes can share one IR log.

Install the optional dependency::

    pip install "psycopg[binary]>=3"
    # or: pip install -e ".[postgres]"

Connection strings come from ``DATABASE_URL`` / ``TRAJIR_DATABASE_URL`` or an
explicit DSN. Never commit credentials.
"""

from drivers.postgres.log import PostgresNodeLog, open_postgres_node_log

__all__ = [
    "PostgresNodeLog",
    "open_postgres_node_log",
]
