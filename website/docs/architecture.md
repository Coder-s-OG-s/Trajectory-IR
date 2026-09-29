# Architecture (talk-sized)

## One diagram

```text
+------------------------------------------------------------------+
| Host applications / MCP clients                                  |
+------------------------+--------------------+--------------------+
| Go client SDK          | Python client SDK  | MCP stdio tools    |
+------------------------+--------------------+--------------------+
| Semantics core: nodes, seals, effects, resume matrix, projector  |
+------------------------+--------------------+--------------------+
| Durable adapters       | NodeLog / CAS      | Package (.tir) IO  |
| Temporal | DBOS        | SQLite / Postgres  | thin / fat / sign  |
+------------------------+--------------------+--------------------+
| External engines and object stores                               |
+------------------------------------------------------------------+
```

## Boundary (say this out loud)

| Layer | Owns |
|---|---|
| Temporal / DBOS / Restate | Crash detection, retries, leases, durable memo |
| Trajectory IR | Node identity, seals, effect classes, at-most-one gate, `.tir` portability |
| Host app | Models, tools, product UX |

## Non-goals (protect the pitch)

- Not LangGraph / agent graph orchestration
- Not a multi-tenant SaaS control plane
- Not a long-term memory product
- Not a reimplementation of Temporal
- Not exactly-once remote writes (forward the seal-derived key; the server honors it)
- Not a process sandbox or a `bash` AST
- Not a compiler IR; `.tir` is a runtime trajectory package

Normative text: [README.md](https://github.com/Coder-s-OG-s/Trajectory-IR/blob/main/README.md), [SCOPE_AND_NON_GOALS.md](https://github.com/Coder-s-OG-s/Trajectory-IR/blob/main/docs/SCOPE_AND_NON_GOALS.md).
