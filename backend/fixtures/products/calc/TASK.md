# calc

Build a command-line calculator. It reads **statements from stdin, one per line**, and prints the value of
every expression. Think of a tiny REPL with variables, where wrong input must be reported precisely and
never crash the tool.

## Contract

- The entry point is `main.py` at the root of your zip. It is started as `python3 main.py` (no arguments)
  with the Python standard library only (Python 3.12, no network, no pip).
- Every line is blank, a comment, an assignment or an expression. Comments start with `#` and run to the end
  of the line (they may follow a statement). Blank and comment-only lines print nothing. Lines end with `\n`
  or `\r\n`; spaces and tabs between tokens are ignored.
- **Expression** statement: print its value on one line (format below).
- **Assignment** `name = expression`: print nothing, remember the value. The right side sees the old value
  of `name`, so `x = x + 1` works. A failed assignment leaves the variable unchanged.

### Language

Numbers are IEEE doubles. A number is digits with an optional fraction and exponent: `12`, `3.5`, `.5`, `5.`,
`1e3`, `1.5E-3`, `2e+4`. Names are `[A-Za-z_][A-Za-z0-9_]*`, case-sensitive. `pi` and `e` are predefined
constants and cannot be assigned. Function names (below) cannot be assigned or used without a call.

Operators, from loosest to tightest:

| operators | notes |
|-----------|-------|
| `+` `-` | left to right |
| `*` `/` `%` | left to right; `%` is the *floored* modulo: the result has the sign of the divisor (`-7 % 3` is 2, `7 % -3` is -2, `5.5 % 2` is 1.5) |
| unary `-` `+` | `-3 % 5` is `(-3) % 5` |
| `^` | power, **right to left**: `2 ^ 3 ^ 2` is 512; binds tighter than unary minus on its left (`-2 ^ 2` is -4); its exponent may be signed (`2 ^ -2` is 0.25) |

Parentheses group. Functions: `sqrt(x)`, `abs(x)`, `floor(x)`, `ceil(x)` (exactly one argument) and `min(...)`,
`max(...)` (one or more). There is no implicit multiplication: `2(3)`, `2 3` and `2pi` are errors.

### Output format

Print a value as follows: `0` for zero (also for -0); a whole number below 10^15 in magnitude as plain digits
without a decimal point; anything else exactly as Python's `format(x, ".12g")`. So `0.1 + 0.2` prints `0.3`,
`1 / 3` prints `0.333333333333`, `1e15` prints `1e+15` and `1e-7` prints `1e-07`.

### Errors

A statement that is not valid (bad character, unbalanced parentheses, missing operand, unknown name or
function, wrong number of arguments, assigning to a constant or function, a lone `=`, a chained `x = y = 1`)
or cannot be evaluated (division or modulo by zero, `sqrt` of a negative number, a power with no real result
such as `(-8) ^ 0.5` or `0 ^ -1`, any result or literal that is not finite, e.g. `1e999` or `10 ^ 400`)
prints `error line N` (N is the 1-based line number, counting blank and comment lines) instead of a value. The
run goes on with the next line. The exit code is **0** if no line failed, **1** otherwise.

### Limits

Lines can be huge: 10 000 terms in a sum, 5 000 nested parentheses, a chain of 5 000 unary minus signs,
2 000 nested function calls, a power chain of 2 000 terms, thousands of statements. Each run has 10
seconds, so avoid both quadratic loops and recursion that hits Python's call-stack limit.

## Scoring

Your tool is run against a set of scenarios (stdin, expected stdout and exit code): precedence and
associativity, number formatting, every function, variables, every kind of error with exact line numbers, and
the large inputs above. Your score is the share of scenarios that pass. After the deadline all entries are
published and people vote on them. You can upload up to 3 times; your best run counts.
