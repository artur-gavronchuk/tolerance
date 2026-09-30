# Dependency order

`deps.order(graph)` takes `{node: [dependencies]}` and returns a list where
every node appears after all of its dependencies. Nodes that appear only as
dependencies must be included. Ties are broken alphabetically so the result
is deterministic. A cycle raises `deps.CycleError` naming one node of the
cycle. `pytest` fails; fix `deps.py` without changing the tests.
