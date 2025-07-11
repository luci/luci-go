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

# Default values
RUNS=50
VERBOSE=false
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
  echo "Runs a test suite multiple times to check for flakiness."
  echo ""
  echo "Options:"
  echo "  -h, --help      Show this help message."
  echo "  -v, --verbose   Show the full output of each test run."
  echo "  -n, --runs      The number of times to run the test (default: 100)."
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
    AVG_TIME=$(echo "scale=2; $ELAPSED / ($i - 1)" | bc)
    REMAINING=$(echo "scale=0; ($RUNS - $i + 1) * $AVG_TIME / 1" | bc)
    REMAINING_MINS=$((REMAINING / 60))
    REMAINING_SECS=$((REMAINING % 60))
    ESTIMATE=" (est. ${REMAINING_MINS}m ${REMAINING_SECS}s remaining)"
  else
    ESTIMATE=""
  fi

  echo -ne "Running $i/$RUNS: ${GREEN}$PASSES passed${NC}, ${RED}$FAILURES failed${NC} ${PROGRESS_BAR}${ESTIMATE}"

  if [ "$VERBOSE" = true ]; then
    npm test -- "$MATCHER"
  else
    npm test -- "$MATCHER" >/dev/null 2>&1
  fi

  if [ $? -ne 0 ]; then
    FAILURES=$((FAILURES + 1))
  else
    PASSES=$((PASSES + 1))
  fi
done

# Final summary
echo -e "\n"
echo "Flakiness check complete."
echo -e "Result: ${GREEN}$PASSES passed${NC}, ${RED}$FAILURES failed${NC} out of $RUNS runs."

if [ $FAILURES -gt 0 ]; then
  exit 1
else
  exit 0
fi
