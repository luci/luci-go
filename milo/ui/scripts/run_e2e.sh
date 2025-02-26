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
vite preview & pid_preview_server=($!)
trap "kill $pid_preview_server" INT TERM EXIT

cypress run --no-runner-ui
