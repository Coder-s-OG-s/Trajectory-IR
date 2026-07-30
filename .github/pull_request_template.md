## Summary

<!-- What changed and why. A few sentences is enough. -->

## Related issues

<!-- Example: Closes #123  or  Part of #2 -->

## How I tested

<!-- Commands you ran, or "docs only" / "templates only". -->

```bash
# example
pip install -e ".[dev]"
pytest
ruff check pkg test
```

## Checklist

- [ ] Commits are DCO signed (`git commit -s`)
- [ ] Change matches the master README / `spec/` (no invented APIs)
- [ ] CI relevant checks pass locally when this is not a docs-only PR
- [ ] AI assistance disclosed below if a tool wrote a meaningful share of the change

## Safety areas

Does this touch effect classes, resume / block-and-gate, seals, or secret handling?

- [ ] No
- [ ] Yes (call it out here; expects careful review)

## AI assistance (if any)

<!-- Example: Assisted with Cursor for boilerplate; I reviewed all diffs. Or: None. -->
