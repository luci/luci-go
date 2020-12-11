# Regression Test Selection (RTS)

Regression Test Selection (RTS) is a technique to intellegently select tests to
run, without spending too much resources on testing, but still detecting bad
code changes. Conceptually, an RTS strategy for CQ accepts changed files as
input and returns tests to run as output.

## Evaluation

Selection strategy evaluation is a process of measuring the candidate strategy's
*safety* and *efficiency*. It is mandatory before deploying the candidate
strategy into production. Read more in [doc/eval.md](./doc/eval.md).
