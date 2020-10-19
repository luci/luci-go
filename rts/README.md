# Regression Test Selection (RTS)

Regression Test Selection (RTS) is a technique to intellegently select tests to
run, such that bad code is detected, but without spending too much resources on
testing.
Conceptually, an RTS algorithm for CQ accepts changed files as input and
returns tests to run as output.

## Evaluation

RTS algorithm evaluation is a process of measuring a candidate algorithm's
*safety* and *efficiency*. It is mandatory before deploying a candidate
algorithm into production. Read more in [EVAL.md](./doc/eval.md).
