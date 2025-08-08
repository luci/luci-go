#!/bin/bash
# Copyright 2025 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

# Default values
RUNS=50
VERBOSE=false
PERF=false
MATCHER=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Help message
function show_help {
  echo "Usage: $0 [options] <test_matcher>"
  echo ""
  echo "Runs a test suite multiple times to check for flakiness and performance."
  echo ""
  echo "Options:"
  echo "  -h, --help      Show this help message."
  echo "  -v, --verbose   Show the full output of each test run."
  echo "  -n, --runs      The number of times to run the test (default: 50)."
  echo "  -p, --perf      Collect and display performance metrics."
}

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    -h|--help)
      show_help
      exit 0
      ;;
    -v|--verbose)
      VERBOSE=true
      shift
      ;;
    -n|--runs)
      RUNS="$2"
      shift
      shift
      ;;
    -p|--perf)
      PERF=true
      shift
      ;;
    *)
      MATCHER="$1"
      shift
      ;;
  esac
done

# Check if a test matcher was provided
if [ -z "$MATCHER" ]; then
  echo -e "${RED}Error: No test matcher provided.${NC}"
  echo ""
  show_help
  exit 1
fi

# Run the tests
FAILURES=0
PASSES=0
START_TIME=$(date +%s)
TIMINGS_FILE=$(mktemp)
ALL_TIMINGS_FILE=$(mktemp)

# Get the directory of the ui folder.
UI_DIR=$(dirname "$(dirname "$0")")

for i in $(seq 1 $RUNS); do
  # Clear the line
  echo -ne "\r\033[K"

  # Progress bar
  PROGRESS_BAR="["
  for j in $(seq 1 $PASSES); do
    PROGRESS_BAR="${PROGRESS_BAR}${GREEN}#"
  done
  for j in $(seq 1 $FAILURES); do
    PROGRESS_BAR="${PROGRESS_BAR}${RED}#"
  done
  for j in $(seq 1 $((RUNS - i + 1))); do
    PROGRESS_BAR="${PROGRESS_BAR} "
  done
  PROGRESS_BAR="${PROGRESS_BAR}]"

  # Time estimation
  NOW=$(date +%s)
  ELAPSED=$((NOW - START_TIME))
  if [ $i -gt 1 ]; then
    AVG_TIME=$(awk "BEGIN { print $ELAPSED / ($i - 1) }")
    REMAINING=$(awk "BEGIN { print int(($RUNS - $i + 1) * $AVG_TIME) }")
    REMAINING_MINS=$((REMAINING / 60))
    REMAINING_SECS=$((REMAINING % 60))
    ESTIMATE=" (est. ${REMAINING_MINS}m ${REMAINING_SECS}s remaining)"
  else
    ESTIMATE=""
  fi

  echo -ne "Running $i/$RUNS: ${GREEN}$PASSES passed${NC}, ${RED}$FAILURES failed${NC} ${PROGRESS_BAR}${ESTIMATE}"

  output=$(cd "$UI_DIR" && npm test -- "$MATCHER" 2>&1)
  exit_code=$?

  if [ "$VERBOSE" = true ]; then
    echo "$output"
  fi

  if [ $exit_code -ne 0 ]; then
    FAILURES=$((FAILURES + 1))
  else
    PASSES=$((PASSES + 1))
    if [ "$PERF" = true ]; then
      # Extract timings for the top 10 slowest tests
      echo "$output" | awk '
        /Top [0-9]+ slowest examples/,/Test Suites:/ {
          if ($0 ~ /<.+>/) {
            test_name=$0
          }
          if ($0 ~ /[0-9.]+ seconds/) {
            time=$1
            gsub(/s/, "", time)
            gsub(/^[ \t]*/, "", test_name)
            print test_name "|" time
          }
        }
      ' >> "$TIMINGS_FILE"

      # Extract total run time
      run_time_s=$(echo "$output" | grep -oE 'Time:\s+[0-9.]+\s?s' | awk '{print $2}')
      if [ -n "$run_time_s" ]; then
        run_time_ms=$(awk -v time_s="$run_time_s" 'BEGIN { print time_s * 1000 }')
        echo "$run_time_ms" >> "$ALL_TIMINGS_FILE"
      fi
    fi
  fi
done

# Final summary
echo -e "\n"
echo "Test check complete."
echo -e "Result: ${GREEN}$PASSES passed${NC}, ${RED}$FAILURES failed${NC} out of $RUNS runs."

if [ "$PERF" = true ]; then
  # Calculate and display average timings
  echo -e "\n${YELLOW}Average timings for the slowest tests:${NC}"
  awk -F'|' '{ gsub(/s/, "", $2); total[$1] += $2; count[$1]++ } END { for (test in total) { printf "%.3fs\t%s\n", total[test]/count[test], test } }' "$TIMINGS_FILE" | sort -rn

  # Calculate and display overall stats
  echo -e "\n${YELLOW}Overall test performance metrics:${NC}"
  lines=$(wc -l < "$ALL_TIMINGS_FILE")
  if [ "$lines" -gt 0 ]; then
    avg_time_ms=$(awk '{ total += $1; count++ } END { print total/count }' "$ALL_TIMINGS_FILE")
    avg_time_s=$(awk "BEGIN { print $avg_time_ms / 1000 }")
    echo -e "Average: ${avg_time_s}s"

    sorted_timings=$(mktemp)
    sort -n "$ALL_TIMINGS_FILE" > "$sorted_timings"

    p50_index=$(awk -v lines="$lines" 'BEGIN { print int(lines * 0.5) }')
    p90_index=$(awk -v lines="$lines" 'BEGIN { print int(lines * 0.9) }')
    p99_index=$(awk -v lines="$lines" 'BEGIN { print int(lines * 0.99) }')

    # Handle case where index is 0
    [ "$p50_index" -eq 0 ] && p50_index=1
    [ "$p90_index" -eq 0 ] && p90_index=1
    [ "$p99_index" -eq 0 ] && p99_index=1

    p50_val_ms=$(sed -n "${p50_index}p" "$sorted_timings")
    p90_val_ms=$(sed -n "${p90_index}p" "$sorted_timings")
    p99_val_ms=$(sed -n "${p99_index}p" "$sorted_timings")

    p50_val_s=$(awk "BEGIN { print $p50_val_ms / 1000 }")
    p90_val_s=$(awk "BEGIN { print $p90_val_ms / 1000 }")
    p99_val_s=$(awk "BEGIN { print $p99_val_ms / 1000 }")

    echo -e "p50 (median): ${p50_val_s}s"
    echo -e "p90: ${p90_val_s}s"
    echo -e "p99: ${p99_val_s}s"

    rm "$sorted_timings"
  fi
fi

rm "$TIMINGS_FILE"
rm "$ALL_TIMINGS_FILE"

if [ $FAILURES -gt 0 ]; then
  exit 1
else
  exit 0
fi
