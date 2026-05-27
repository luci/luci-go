#!/bin/bash
# Copyright 2024 The LUCI Authors.
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
[ -f run_e2e.sh ] || die 'failed to find own directory'
cd ../

# Run the preview server in the background.
PORT=${CYPRESS_PORT:-8001}
npx vite preview --port $PORT --host 127.0.0.1 & pid_preview_server=($!)
echo "Starting E2E tests on port: $PORT (Override using CYPRESS_PORT if conflicted)"
trap "kill $pid_preview_server" INT TERM EXIT

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

CYPRESS_BASE_URL=http://localhost:$PORT npx cypress run
