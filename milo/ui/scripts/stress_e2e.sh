#!/bin/bash
# Copyright 2026 The LUCI Authors.
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

die() {
  printf '%s\n' "$@" 1>&2
  exit 1
}

cd -- "$(dirname "$0")" || die 'cannot chdir'
[ -f stress_e2e.sh ] || die 'failed to find own directory'
cd ../

# Stress parameters
ITERATIONS=${STRESS_COUNT:-10}
SPEC=${STRESS_SPEC:-""}

echo "Building project to ensure fresh assets..."
npm run build || die "Failed to build project"

PORT=${PREVIEW_PORT:-8002}
npx vite preview --port $PORT --host 127.0.0.1 --strictPort & pid_preview_server=$!
echo "Starting E2E stress tests on port: $PORT ($ITERATIONS iterations)"
trap "kill \$pid_preview_server 2>/dev/null || true" INT TERM EXIT

# Wait for the server to be ready
echo "Waiting for server to be ready on port $PORT..."
for i in {1..30}; do
  if curl -s http://127.0.0.1:$PORT > /dev/null; then
    echo "Server is ready!"
    break
  fi
  if [ $i -eq 30 ]; then
    echo "Server failed to start on port $PORT"
    exit 1
  fi
  sleep 1
done

PASSED=0
FAILED=0
FAILED_RUNS=""

for ((run=1; run<=ITERATIONS; run++)); do
  echo "===================================================="
  echo "STRESS RUN $run OF $ITERATIONS"
  echo "===================================================="
  
  if [ -n "$SPEC" ]; then
    CYPRESS_BASE_URL=http://127.0.0.1:$PORT npx cypress run --spec "$SPEC"
  else
    CYPRESS_BASE_URL=http://127.0.0.1:$PORT npx cypress run
  fi
  
  STATUS=$?
  if [ $STATUS -eq 0 ]; then
    PASSED=$((PASSED + 1))
    echo "STRESS RUN $run: PASSED"
  else
    FAILED=$((FAILED + 1))
    FAILED_RUNS="$FAILED_RUNS $run"
    echo "STRESS RUN $run: FAILED (exit code $STATUS)"
  fi
done

echo "===================================================="
echo "STRESS TEST RESULTS SUMMARY"
echo "===================================================="
echo "Total Iterations: $ITERATIONS"
echo "Passed: $PASSED"
echo "Failed: $FAILED"

if [ $FAILED -gt 0 ]; then
  echo "Failed on runs: $FAILED_RUNS"
  exit 1
else
  echo "All runs passed! The spec is robust and verified non-flaky."
  exit 0
fi
