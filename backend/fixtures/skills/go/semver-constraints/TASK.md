# Semantic version constraints

Package `semver` parses versions and constraints and picks the best release
from a list (`Highest`, `Matching`). The visible tests pass, but real users
report wrong resolutions. Fix the package without editing `*_test.go` files
or adding dependencies. Hidden tests exercise the contract below.

## Versions

`MAJOR.MINOR.PATCH[-PRE][+BUILD]`, with an optional leading `v` or `V`.

- Numbers are decimal without leading zeros (`0` is fine, `01` is not) and must
  fit a `uint64`. All three parts are required.
- PRE is dot-separated identifiers of `[0-9A-Za-z-]`, none empty; a purely
  numeric identifier has no leading zeros (`1.0.0-0` is fine, `1.0.0-01` is not).
- BUILD is dot-separated non-empty identifiers of `[0-9A-Za-z-]`; leading zeros
  are allowed there.
- Anything else is an error wrapping `ErrSyntax`.

## Precedence (`Compare`)

Major, minor, patch compare numerically. A version with a pre-release is lower
than the same version without one. Two pre-releases compare identifier by
identifier, left to right: numeric identifiers compare as numbers (even when
longer than a `uint64`), alphanumeric ones compare as ASCII text, a numeric
identifier is lower than an alphanumeric one, and when all shared identifiers
are equal the one with fewer identifiers is lower. **Build metadata is ignored**:
`1.0.0+a` and `1.0.0+b` have equal precedence.

## Constraints

Alternatives are separated by `||`; inside an alternative, comparators are
separated by commas and/or whitespace and must all hold. An operator may be
separated from its version by whitespace (`>= 1.2.3`). An empty alternative is
an `ErrSyntax` error.

A comparator is an optional operator and a version that may be *partial*
(`1`, `1.2`) or contain a wildcard (`x`, `X`, `*` in place of the trailing parts;
nothing may follow a wildcard). A partial version carries a pre-release only if it is
complete.

| comparator | meaning |
| --- | --- |
| `1.2.3`, `=1.2.3` | exactly that precedence (build ignored) |
| `1.2`, `=1.2`, `1.2.x` | `>=1.2.0 <1.3.0` |
| `1`, `1.x` | `>=1.0.0 <2.0.0` |
| `*`, `x` | any release |
| `!=1.2.3` | not that version (a full version is required) |
| `>1.2.3` / `>=1.2.3` / `<1.2.3` / `<=1.2.3` | as written |
| `>1.2` | `>=1.3.0` (above everything in 1.2.x); `>1` is `>=2.0.0` |
| `>=1.2` | `>=1.2.0`; `<1.2` is `<1.2.0` |
| `<=1.2` | `<1.3.0` (everything in 1.2.x is included); `<=1` is `<2.0.0` |
| `~1.2.3`, `~1.2` | `>=1.2.3 <1.3.0` and `>=1.2.0 <1.3.0` |
| `~1` | `>=1.0.0 <2.0.0` |
| `^1.2.3`, `^1.2`, `^1` | `<2.0.0` above the given lower bound |
| `^0.2.3`, `^0.2` | `<0.3.0` |
| `^0.0.3` | `<0.0.4` |
| `^0.0` | `<0.1.0`; `^0` and `^0.x` are `<1.0.0` |

In short, `^` keeps the left-most non-zero part among the parts that were
written; `~` allows patch-level changes when a minor is given, minor-level
changes otherwise. Missing parts of a lower bound are zero. Operators other than
`=` and bare versions need a version (`>=`, `~` alone or `>*` are errors).
A pre-release on a bound is kept (`>=1.2.3-beta.2`, `~1.2.3-beta.1`).

**Pre-releases.** A version that has a pre-release satisfies an alternative
only if, besides satisfying every comparator numerically, at least one
comparator *of that same alternative* carries a pre-release on the same
`MAJOR.MINOR.PATCH`. So `>=1.2.3` and `^1.2.3` never accept `1.4.0-beta`, nor
`2.0.0-alpha` for `<2.0.0`, while `>=1.2.3-beta.2 <2.0.0` accepts
`1.2.3-rc.1` but not `1.3.0-alpha`. The rule is evaluated per alternative.

## Resolution

`Matching` returns the input strings (exactly as given) that parse and satisfy
the constraint, ascending by precedence; versions of equal precedence keep their
input order. Strings that do not parse are skipped; a bad constraint is an
error. `Highest` returns the highest-precedence match; among versions of equal
precedence the one listed first wins. `ErrNoMatch` if nothing matches.
