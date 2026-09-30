import pytest

from deps import CycleError, order


def test_hidden_dependency_only_nodes_included():
    assert order({"app": ["lib"]}) == ["lib", "app"]


def test_hidden_alphabetical_ties():
    assert order({"b": [], "a": [], "c": ["a", "b"]}) == ["a", "b", "c"]


def test_hidden_cycle_inside_larger_graph():
    with pytest.raises(CycleError):
        order({"app": ["lib"], "lib": ["util"], "util": ["lib"], "docs": []})


def test_hidden_self_cycle():
    with pytest.raises(CycleError):
        order({"a": ["a"]})


def test_hidden_cycle_error_names_a_node():
    with pytest.raises(CycleError) as e:
        order({"x": ["y"], "y": ["z"], "z": ["x"]})
    assert any(n in str(e.value) for n in ("x", "y", "z"))
