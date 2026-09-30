# Sliding-window rate limiter

`limiter.RateLimiter(limit, window_seconds)` has `allow(key, now) -> bool`.
A key may be allowed at most `limit` times in any window of
`window_seconds` seconds (sliding, not fixed buckets). `now` is a float
timestamp supplied by the caller. Old hits must be forgotten so memory does
not grow. `pytest` fails; fix `limiter.py` without changing the tests.
