# RTS evaluation

RTS algorithm evaluation is a process of measuring a candidate algorithm's
*safety* and *efficiency*. It is done by emulating CQ behavior **with** the
candidate algorithm, based on CQ's historial records.

[TOC]

## Safety

A safe RTS algorithm doesn't let bad code into the repository.
There are two safety scores: change recall and test recall.

### Change recall

Change recall is the ratio `preserved_rejections/total_rejections`, where
*   `total_rejections` is the number of patchsets rejected by CQ due to test
    failures. More generally, a single *rejection* may span multiple patchsets.
*  `preserved_rejections` is how many of them would still be rejected
    if the candidate RTS algorithm was deployed.

A rejection is considered *preserved* if and only if the RTS algorithm selects
at least one test that caused the rejection. For example, if 10 tests failed out
of 1000, it is sufficient to select just 2 to preserve the rejection.

### Test recall

Test recall is the ratio `preserved_test_failures/total_test_failures`, where
*   `total_test_failures` is the number of test failures that caused a CQ
    rejection.
*   `preserved_test_failures` is how many of them the RTS algorithm would
    preserve. If the algorithm does not select a test, its failure is not
    preserved.

## Efficiency

An efficient RTS algorithm reduces resources spent on testing.
Efficiency is scored as a ratio `saved_duration/total_duration`, where

*   `total_duration` is the sum of test durations found in the historical
    records.
*   `saved_duration` is the duration sum for those tests that the RTS
    algorithm did not select.

## Why evaluate

* Evaluation is mandatory before deploying an RTS algorithm into production;
  otherwise it might let bad code into the repository and create a havoc for
  sheriffs. Evaluation predicts what would happen if the candidate algorithm
  was deployed.
* There are many possible RTS algorithms and we need objective metrics to rank
  them.
* An algorithm developer needs objective metrics in order to fine tune the
  algorithm.

## History files

`.hist` files contain CQ historical records
([file format](https://go.chromium.org/luci/rts/presubmit/eval/history)).
Subcommand `rts-chromium presubmit-history` can be used to fetch history
files for Chromium:

```bash
# Inside infra's go env: https://chromium.googlesource.com/infra/infra/+/master/go/README.md
go install go.chromium.org/luci/rts/cmd/rts-chromium
rts-chromium presubmit-history \
  -from 2020-10-01 -to 2020-10-02 \
  -out 2020-10-01.hist
```

This fetches historical records for 2020-10-01. This example is purely
illustrative; the sections below provide much more useful examples.

## Evaluation framework

To evaluate an algorithm, it must be plugged into the
[evaluation framework](https://go.chromium.org/luci/rts/presubmit/eval). It compiles into an executable that accepts a `.hist` file, emulates CQ by
playing the history back and prints safety/efficiency scores.

There are two examples of framework usages:

* [rts-random](../cmd/rts-random) flips a coin to decide whether to run a
  test. It is a Hello World of RTS algorithms.
* [rts-chromium](../cmd/rts-chromium/eval.go) is Chromium's RTS
  algorithm.

To evaluate `rts-random`:
```bash
go run ../cmd/rts-random/main.go -history 2020-10-01.hist
```

As it runs, it will print all unpreserved rejections as they are discovered.
When done, it will print something like this:

```
Change recall:
  Score: 42%
  # of eligible rejections: 846
  # of them preserved by this RTS: 354
Efficiency:
  Saved: 50%
  Compute time in the sample: 165h32m48.580086037s
  Forecasted compute time: 82h37m32.134879486s
Total records: 2444461
```

## Practical tips

### Safety first

Safety should be evaluated before efficiency because:
* Safety evaluation is much faster because there is less data to analyze.
* The output of safety evaluation is **actionable**, unlike efficiency.
  This is because safety evaluation prints the CLs where the algorithm
  failed to select affected tests.
* Data for efficiency evaluation takes much longer to fetch.

Fine tune your algorithm until it reaches a satisfactory safety score;
then evaluate efficiency. To evaluate only safety, tell the
`presubmit-history` subcommand not to fetch test durations by passing
`-duration-data-frac 0`. As a convention, use `.safety.hist` extension for
history files.
TODO(nodir): define "satisfactory safety score", or perhaps switch to machine
learning.

To iterate even quicker, narrow the evaluation down to one builder using
`-builder` flag.

Omitting test durations and reducing the scope makes it practical to use much wider time window.
The following example fetches all rejections from September 2020:

```bash
rts-chromium presubmit-history \
  -from 2020-09-01 -to 2020-10-01 \
  -data-duration-frac 0 \
  -builder linux-rel \
  -out linux-rel-2020-sept.safety.hist
```

### Safety at the builder level

Some builders, such as linux-rel, are used by developers as representative
of other builders.
In such case, it is important to *preserve rejections* at the builder level,
in addition to CQ at large. For each such builder, produce a `.safety.hist`
file using `-builder` flag. Ensure the algorithm is safe for each file.

### Do not perfect safety

Ideally the algorithm preserves all rejections, but it is often impractical
because sometimes test flakes creep in, despite the efforts to exclude them.
The recommended strategy is to preserve most rejections and manually analyze the rest.
Some of them will turn out to be "OK", and others might provide insight into
further tuning.

### Efficiency evaluation

Efficiency is evaluated only after you are satisfied with safety.

Efficiency and safety evaluations do not have to use the exact same
time window. For example, it would be excessive to fetch all of September's
test durations, given that there is ~1B per day. Instead, consider fetching
only the last day of the time window that was used for safety evaluation.

To speed up efficiency evaluation, play with the following flags:

* `-duration-data-frac` is the fraction of test durations to fetch,
  defaulting to 0.001 (0.1%). Reduce it to get faster and less precise
  evaluation for quicker iteration.
* `-min-duration` is the minimum duration to fetch (default is 1s). In practice,
  quick tests are dwarfed by slow tests. Increase it to have focus on slow tests.

