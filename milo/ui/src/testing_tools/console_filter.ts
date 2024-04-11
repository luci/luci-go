// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/* eslint-disable no-console */

const recoverCallbacks: Array<() => void> = [];

/**
 * Silence console logs that matches the filter in the given mode.
 */
export function silence(
  mode: 'log' | 'warn' | 'error',
  filter: (...params: unknown[]) => boolean,
) {
  const original = console[mode];
  let active = true;
  const filtered = (...params: unknown[]) => {
    if (active && filter(...params)) {
      return;
    }
    original.call(console, ...params);
  };

  console[mode] = filtered;
  recoverCallbacks.push(() => {
    // Set this to false in case the method is stored as a standalone reference
    // somewhere.
    active = false;
    if (console[mode] !== filtered) {
      console.warn(
        `\`console.${mode}\` was modified. The modification will be discarded.`,
      );
    }
    console[mode] = original;
  });
}

/**
 * Undo the silence on best effort.
 */
export function resetSilence() {
  while (recoverCallbacks.length > 0) {
    recoverCallbacks.pop()!();
  }
}
