## Summary

<!-- What changed and why. A few sentences is enough. -->

## Related issues

<!-- Example: Closes #123 -->

## How I tested

```bash
# Python reference (if you touched Python)
pip install -e ".[dev]"
pytest test/unit -q
pytest test/e2e -q
pytest conformance/ -q
ruff check pkg drivers client test conformance examples

# Go primary (if you touched go/)
cd go && go test ./... -count=1
```

## Checklist

- [ ] Commits are DCO signed (`git commit -s`)
- [ ] Change matches the master README (no invented APIs)
- [ ] Relevant CI checks pass locally (or will pass on the PR)
- [ ] If you changed `.github/workflows/**`, note it under Safety and expect extra review
- [ ] AI assistance disclosed below if a tool wrote a meaningful share of the change

## Safety areas

Does this touch effect classes, resume / block-and-gate, seals, or secret handling?

- [ ] No
- [ ] Yes (call it out here)

## AI assistance (if any)

<!-- Example: Assisted with Cursor for boilerplate; I reviewed all diffs. Or: None. -->
