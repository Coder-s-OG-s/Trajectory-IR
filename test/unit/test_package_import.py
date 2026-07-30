"""Smoke tests so CI has a real pytest entrypoint before runtime logic lands."""

from __future__ import annotations

import rfc8785

import trajectory_ir


def test_package_version_is_set() -> None:
    assert trajectory_ir.__version__
    assert isinstance(trajectory_ir.__version__, str)


def test_subpackages_import() -> None:
    import trajectory_ir.effects
    import trajectory_ir.package
    import trajectory_ir.resume
    import trajectory_ir.runtime

    assert trajectory_ir.runtime.__doc__ is not None
    assert trajectory_ir.resume.__doc__ is not None
    assert trajectory_ir.effects.__doc__ is not None
    assert trajectory_ir.package.__doc__ is not None


def test_rfc8785_key_order_stable() -> None:
    # Phase 1A depends on real JCS, not ad hoc sort_keys dumps.
    assert rfc8785.dumps({"b": 1, "a": 2}) == b'{"a":2,"b":1}'
