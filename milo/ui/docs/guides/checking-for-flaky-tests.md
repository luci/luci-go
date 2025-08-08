# Checking for Flaky or Slow Tests

This guide explains how to use the `check_tests.sh` script to identify flaky and slow tests in the UI codebase.

## Overview

The `check_tests.sh` script is a utility that runs a specified test suite multiple times and reports on any failures and/or performance. This is useful for detecting tests that pass and fail intermittently (flaky tests), and for identifying tests that are taking a long time to run.

## Usage

The script is located in the `milo/ui/scripts` directory. You can run it from the root of the `luci-go` repository like this:

```bash
./milo/ui/scripts/check_tests.sh [options] <test_matcher>
```

### Arguments

* `<test_matcher>`: (Required) A string that matches the test suite(s) you want to run. This is the same matcher you would pass to `npm test`.

### Options

* `-h, --help`: Show the help message.
* `-v, --verbose`: Show the full output of each test run. By default, the output is hidden to keep the progress display clean.
* `-n, --runs`: The number of times to run the test (default: 50).
* `-p, --perf`: Collect and display performance metrics.

## Example

To run the `overview_tab.test.tsx` test suite 50 times, you would use the following command:

```bash
./milo/ui/scripts/check_tests.sh -n 50 milo/ui/src/clusters/components/cluster/cluster_analysis_section/overview_tab/overview_tab.test.tsx
```

To check the performance of the same test suite, you would add the `-p` flag:

```bash
./milo/ui/scripts/check_tests.sh -n 50 -p milo/ui/src/clusters/components/cluster/cluster_analysis_section/overview_tab/overview_tab.test.tsx
```

## Output

The script will display a progress bar and a summary of the test runs.

### Progress Display

While the tests are running, you will see a line that updates in real-time:

```bash
Running 50/50: 49 passed, 1 failed [##################################################] (est. 0m 0s remaining)
```

This line shows:

* The current run number.
* The number of passed and failed tests so far.
* A progress bar that fills up with green for passes and red for failures.
* An estimated time remaining until all runs are complete.

### Final Summary

When the script has finished, it will print a final summary:

```bash
Test check complete.
Result: 49 passed, 1 failed out of 50 runs.
```

If the `-p` flag was used, it will also display the average timings for the slowest tests:

```text
Average timings for the slowest tests:
3.448s    <DeviceTable /> should preserve the current pagination state and honor new page size for the subsequent pages if the page size changes
2.145s    <DeviceTable /> should navigate between pages properly
...
```

If there were any failures, the script will exit with a non-zero exit code.
