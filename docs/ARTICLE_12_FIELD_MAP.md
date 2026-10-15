# Article 12 field map

This is a map from a `.tir` pack to the questions in Article 12. It is not a compliance claim, not a certificate, and not a retention product. Six months of storage, when it applies, is the customer's store.

The pack is evidence only when the decision was sealed in process before the tool body. A log that arrives after the tool finished does not answer these questions.

| Question | Where it is in the pack | What it is not |
| --- | --- | --- |
| Who acted | `tenant_id` on every node | A user directory or an employer name. The tenant string is whatever the host passed in. |
| When | `ts` on the node row | Part of the content hash. The hash covers the canonical payload, not the wall clock. |
| Which decision | Node kind `DECISION`, sequence 1, payload `tool_calls` | A new plan invented on resume. Resume keeps this payload. |
| Which tool | `TOOL_CALL` payload field `tool` | A guess from a model about a tool that was not named. |
| Which effect class | The class `classify_tool` chose before `exec_tool` | A field inside the hashed tool arguments. Open-world names, including `Bash`, are `NON_IDEMPOTENT_WRITE`. An unknown tool with no MCP hints is the same class. |
| Which idempotency key | `TOOL_CALL` payload field `idempotency_key` | A key injected into the tool arguments. The host may forward it. The wrap does not put it in `args`. |
| Which content hash | `payload_hash` of the decision payload, and the node id derived from it | A signature. Ed25519 package signatures exist separately. Sigstore is not this map. |

`scripts/local_soak.py` can write one filled row on the machine that runs it. That report stays gitignored at `docs/LOCAL_SOAK_REPORT.md`. An absent report means no example row is claimed.

Block-and-gate is at-most-one automatic attempt from this process. It does not prove the remote side received the call.
