from limiter import RateLimiter


def test_allows_up_to_limit():
    rl = RateLimiter(3, 10)
    assert all(rl.allow("k", t) for t in (0, 1, 2))
    assert not rl.allow("k", 3)


def test_window_slides():
    rl = RateLimiter(2, 10)
    assert rl.allow("k", 0)
    assert rl.allow("k", 5)
    assert not rl.allow("k", 9)
    assert rl.allow("k", 11)
