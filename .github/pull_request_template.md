## Summary

<!-- What changed and why. A few sentences is enough. -->

## Related issues

<!-- Example: Closes #123 -->

## How I tested

```bash
pip install -e ".[dev]"
pytest test/unit -q
pytest test/e2e -q
pytest conformance/ -q
ruff check pkg drivers client test conformance examples
```

## Checklist

- [ ] Commits are DCO signed (`git commit -s`)
- [ ] Change matches the master README (no invented APIs)
- [ ] Relevant CI checks pass locally
- [ ] AI assistance disclosed below if a tool wrote a meaningful share of the change

## Safety areas

Does this touch effect classes, resume / block-and-gate, seals, or secret handling?

- [ ] No
- [ ] Yes (call it out here)

## AI assistance (if any)

<!-- Example: Assisted with Cursor for boilerplate; I reviewed all diffs. Or: None. -->
