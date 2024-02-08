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

/**
 * Get the longest common prefix of the strings. This is not optimized to
 * handle large datasets.
 */
export function getLongestCommonPrefix(strings: readonly string[]): string {
  if (!strings.length) {
    return '';
  }

  const baseStr = strings[0];
  let commonPrefixLength = baseStr.length;
  for (let i = 1; i < strings.length; ++i) {
    const compareStr = strings[i];
    for (let j = 0; j < commonPrefixLength; ++j) {
      if (baseStr[j] !== compareStr[j]) {
        commonPrefixLength = j;
        break;
      }
    }
  }
  return baseStr.slice(0, commonPrefixLength);
}
