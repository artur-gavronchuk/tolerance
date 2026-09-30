"""Sliding-window rate limiter."""
from collections import defaultdict, deque


class RateLimiter:
    def __init__(self, limit: int, window_seconds: float):
        self.limit = limit
        self.window = window_seconds
        self._hits: dict[str, deque[float]] = defaultdict(deque)

    def allow(self, key: str, now: float) -> bool:
        hits = self._hits[key]
        while hits and hits[0] < now - self.window:
            hits.popleft()
        hits.append(now)
        return len(hits) <= self.limit
