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

/**
 * Get the longest common substring in a list of strings. This is not
 * optimized to handle large datasets.
 */
export const getLongestCommonSubstring = (
  strings: readonly string[],
): string => {
  if (!strings.length) {
    return '';
  }
  if (strings.length === 1) {
    return strings[0];
  }
  const shortest = strings.reduce((a, b) => (a.length <= b.length ? a : b));
  let longest = '';
  for (let i = 0; i < shortest.length; i++) {
    for (let j = i + 1; j <= shortest.length; j++) {
      const sub = shortest.substring(i, j);
      if (strings.every((str) => str.includes(sub))) {
        if (sub.length > longest.length) {
          longest = sub;
        }
      }
    }
  }
  return longest;
};

/**
 * Format number with a cap. If the number is greater than `cap`, display
 * `${cap}+` instead. This is useful when displaying a large number in limited
 * space.
 */
export function formatNum(num: number, hasMore: boolean, cap?: number) {
  if (cap && num > cap) {
    return `${cap}+`;
  } else if (hasMore) {
    return `${num}+`;
  }
  return `${num}`;
}

/**
 * Hashes a string to a number, it uses the Java style hashing algorithm.
 *
 * Note: This method can return negative values as it uses bit shifts.
 */
export function hashStringToNum(str: string) {
  return str.split('').reduce((acc, char) => {
    const charCode = char.charCodeAt(0);
    return (acc << 5) - acc + charCode;
  }, 0);
}
