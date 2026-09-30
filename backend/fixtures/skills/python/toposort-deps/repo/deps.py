"""Deterministic topological order of a dependency graph."""


class CycleError(ValueError):
    pass


def order(graph: dict[str, list[str]]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []

    def visit(node: str) -> None:
        if node in seen:
            return
        seen.add(node)
        for dep in sorted(graph.get(node, [])):
            visit(dep)
        out.append(node)

    for node in sorted(graph):
        visit(node)
    return out
