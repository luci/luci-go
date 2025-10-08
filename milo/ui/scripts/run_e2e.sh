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

oldpwd="$PWD"
nominal_self_dir="$(dirname -- "$0")" || die 'failed to determine nominal self dir'
my_basename="$(basename -- "$0")" || die 'failed to get basename'
self="$( cd -P -- "$nominal_self_dir" && printf '%s/%s' "$(pwd -P)" "$my_basename" )" || die 'failed to find self'
selfdir="$(dirname -- "$self")" || die 'cannot find own directory'

cd -P -- "$selfdir" || die 'cannot chdir'

eval "$(../../../../../../env.py)"

cd ../

# Check if any vite-flavored processes are running
if ps aux | grep -q 'vit[e]'; then
  die 'vite is also running right now! cowardly refusing to run cypress'
fi

# Run the preview server in the background.
npx vite preview &
cpid="$!"
cleanup() {
  # This cleans up all vite-flavored processes with a sledgehammer.
  # Any process with the word "vite" appearing anywhere in its output will be
  # terminated.
  #
  # Unfortunately, just doing the basic 'kill -INT "$cpid"' seems not to work.
  #
  # TODO(gregorynisbet): Make this less terrible.
  kill -INT $(ps ax | grep 'vit[e]' | awk '{print $1}')
}
trap "cleanup" INT TERM EXIT

npx cypress run
