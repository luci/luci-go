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

export const MAX_QUOTA = 100_000;

/**
 * Validates that an expected quota input string represents a positive whole integer
 * greater than 0 and within the maximum quota limit (100,000).
 *
 * Returns an error message if invalid, or an empty string if valid.
 */
export const validateQuota = (val: string): string => {
  if (val.trim() === '') {
    return 'Expected quota must be a positive whole integer greater than 0.';
  }
  const num = Number(val);
  if (isNaN(num) || num <= 0 || !Number.isInteger(num)) {
    return 'Expected quota must be a positive whole integer greater than 0.';
  }
  if (num > MAX_QUOTA) {
    return `Expected quota cannot exceed ${MAX_QUOTA.toLocaleString()}.`;
  }
  return '';
};
