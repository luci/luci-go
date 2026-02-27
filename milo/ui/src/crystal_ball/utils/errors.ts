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
 * Parses and formats an API error into a human-readable string.
 * @param error - The raw error thrown by the API or query hook.
 * @param defaultMessage - The message to return if parsing fails.
 * @returns A formatted error string.
 */
export function formatApiError(
  error: unknown,
  defaultMessage = 'An unknown error occurred.',
): string {
  if (!error) return defaultMessage;

  if (typeof error === 'object' && error !== null) {
    const gapiError = error as { result?: { error?: { message?: string } } };
    if (gapiError.result?.error?.message) {
      return gapiError.result.error.message;
    }

    const rawError = error as { error?: { message?: string } };
    if (rawError.error?.message) {
      return rawError.error.message;
    }

    if (error instanceof Error) {
      try {
        const parsed = JSON.parse(error.message);
        if (parsed?.error?.message) {
          return parsed.error.message;
        }
      } catch {
        // Not JSON, ignore
      }
      return error.message;
    }
  }

  return String(error);
}
