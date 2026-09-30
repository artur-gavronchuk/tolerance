from intervals import merge


def test_overlapping():
    assert merge([(1, 4), (2, 5)]) == [(1, 5)]


def test_touching_merge():
    assert merge([(1, 3), (3, 5)]) == [(1, 5)]
