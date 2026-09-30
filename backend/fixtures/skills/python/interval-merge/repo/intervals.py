"""Merge overlapping integer intervals."""


def merge(ranges: list[tuple[int, int]]) -> list[tuple[int, int]]:
    ranges.sort()
    out: list[tuple[int, int]] = []
    for start, end in ranges:
        if out and start < out[-1][1]:
            out[-1] = (out[-1][0], max(out[-1][1], end))
        else:
            out.append((start, end))
    return out
