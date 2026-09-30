import pytest

from deps import CycleError, order


def test_simple_chain():
    assert order({"app": ["lib"], "lib": ["core"], "core": []}) == ["core", "lib", "app"]


def test_cycle_raises():
    with pytest.raises(CycleError):
        order({"a": ["b"], "b": ["a"]})
