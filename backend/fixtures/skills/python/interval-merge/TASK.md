# Merge intervals

`intervals.merge(ranges)` takes a list of `(start, end)` integer pairs with
`start <= end`, in any order, and returns the minimal list of merged
intervals sorted by start. Touching intervals like `(1, 3)` and `(3, 5)`
merge into `(1, 5)`. The input must not be modified. `pytest` fails; fix
`intervals.py` without changing the tests.
