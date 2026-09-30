from limiter import RateLimiter


def test_hidden_denied_calls_do_not_count():
    rl = RateLimiter(2, 10)
    assert rl.allow("k", 0) and rl.allow("k", 1)
    for t in range(2, 9):
        assert not rl.allow("k", t)
    # at t=10.5 the hit at t=0 has expired; only the hit at t=1 remains
    assert rl.allow("k", 10.5)


def test_hidden_keys_are_independent():
    rl = RateLimiter(1, 10)
    assert rl.allow("a", 0)
    assert rl.allow("b", 0)
    assert not rl.allow("a", 1)


def test_hidden_boundary_is_exclusive():
    rl = RateLimiter(1, 10)
    assert rl.allow("k", 0)
    assert not rl.allow("k", 10)
    assert rl.allow("k", 10.01)


def test_hidden_memory_is_bounded():
    rl = RateLimiter(5, 1)
    for t in range(10_000):
        rl.allow("k", float(t))
    assert len(rl._hits["k"]) <= 5
