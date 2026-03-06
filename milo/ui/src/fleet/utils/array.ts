// Copyright 2026 The LUCI Authors.
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

/**
 * Partitions an array into two arrays based on a predicate.
 * Returns a tuple where the first element is an array of items satisfying the predicate,
 * and the second element is an array of items failing the predicate.
 */
export const partition = <T>(
  arr: T[],
  predicate: (item: T) => boolean,
): [T[], T[]] => {
  const pass: T[] = [];
  const fail: T[] = [];
  arr.forEach((item) => (predicate(item) ? pass : fail).push(item));
  return [pass, fail];
};
