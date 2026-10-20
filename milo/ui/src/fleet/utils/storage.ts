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
 * Safely retrieves an item from localStorage without throwing exceptions.
 * Useful in private browsing modes or sandboxed iframes where storage access is restricted.
 */
export function safeLocalStorageGet(
  key: string,
  onError?: (err: unknown) => void,
): string | null {
  try {
    return localStorage.getItem(key);
  } catch (err) {
    onError?.(err);
    return null;
  }
}

/**
 * Safely sets an item in localStorage without throwing exceptions.
 * Returns true if successful, false if an error occurred.
 */
export function safeLocalStorageSet(
  key: string,
  value: string,
  onError?: (err: unknown) => void,
): boolean {
  try {
    localStorage.setItem(key, value);
    return true;
  } catch (err) {
    onError?.(err);
    return false;
  }
}

/**
 * Safely removes an item from localStorage without throwing exceptions.
 * Returns true if successful, false if an error occurred.
 */
export function safeLocalStorageRemove(
  key: string,
  onError?: (err: unknown) => void,
): boolean {
  try {
    localStorage.removeItem(key);
    return true;
  } catch (err) {
    onError?.(err);
    return false;
  }
}
