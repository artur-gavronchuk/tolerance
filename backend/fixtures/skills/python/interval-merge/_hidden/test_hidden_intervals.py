from intervals import merge


def test_hidden_input_not_mutated():
    data = [(5, 6), (1, 2)]
    merge(data)
    assert data == [(5, 6), (1, 2)]


def test_hidden_unsorted_and_nested():
    assert merge([(6, 8), (1, 9), (2, 4)]) == [(1, 9)]


def test_hidden_touching_chain():
    assert merge([(3, 4), (1, 2), (2, 3)]) == [(1, 4)]


def test_hidden_empty_and_single():
    assert merge([]) == []
    assert merge([(3, 3)]) == [(3, 3)]


def test_hidden_points_touching():
    assert merge([(1, 1), (1, 1), (2, 2)]) == [(1, 1), (2, 2)]
