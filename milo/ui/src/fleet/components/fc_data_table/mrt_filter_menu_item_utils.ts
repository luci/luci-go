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
 * Parses a comma-separated string into an array of trimmed strings, Supporting quoted commas.
 *
 * Example:
 *  `'a,b,c'` -> `['a', 'b', 'c']`
 *  `'"a,b",c'` -> `['a,b', 'c']`
 */
export const parseCommaSeparatedText = (value: string): string[] => {
  if (!value) return [];
  const result: string[] = [];
  let current = '';
  let inQuotes = false;

  for (let i = 0; i < value.length; i++) {
    const char = value[i];
    if (char === '"') {
      if (!inQuotes && current.trim() !== '') {
        current += char;
      } else {
        inQuotes = !inQuotes;
        current += char;
      }
    } else if (char === ',' && !inQuotes) {
      result.push(current);
      current = '';
    } else {
      current += char;
    }
  }
  if (inQuotes) {
    current += '"';
  }
  if (current) {
    result.push(current);
  }

  return result.map((v) => v.trim().replace(/^"(.*)"$/, '$1')).filter(Boolean);
};

/**
 * Formats an array of strings into a comma-separated string, quoting strings that contain commas.
 */
export const formatCommaSeparatedText = (
  value: string[] | Set<string>,
): string => {
  return Array.from(value)
    .map((v) => (v.includes(',') ? `"${v}"` : v))
    .join(', ');
};
