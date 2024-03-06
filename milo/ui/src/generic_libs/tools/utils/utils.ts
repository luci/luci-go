// Copyright 2021 The LUCI Authors.
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
 * Extend URL with some helper methods.
 */
export class URLExt extends URL {
  setSearchParam(key: string, value: string) {
    this.searchParams.set(key, value);
    return this;
  }

  /**
   * Remove the key from the search param if the value matches the provided
   * value. This is useful for removing unnecessary search params (e.g. when
   * the value matches the default value anyway).
   */
  removeMatchedParams(params: { [key: string]: string }) {
    for (const [key, value] of Object.entries(params)) {
      const existingValues = this.searchParams.getAll(key);
      if (existingValues.length === 1 && existingValues[0] === value) {
        this.searchParams.delete(key);
      }
    }
    return this;
  }
}

/**
 * A utility function that allowing uses a callback to handle the exception
 * instead of using try-catch. This makes writing functional code easier.
 */
export function unwrapOrElse<T>(fn: () => T, onErr: (err: unknown) => T): T {
  try {
    return fn();
  } catch (e) {
    return onErr(e);
  }
}

/**
 * A utility function that replaces non-null assertion (e.g. `value!`) with a
 * runtime check.
 *
 * incorrect usage of `!` may lead to silent failures (e.g. `value!` might get
 * displayed as `undefined`), or failures that are hard to trace (e.g. when
 * `value!` is assigned to another variable and passed around, leading to errors
 * in a place far away from the incorrect usage of `!`).
 *
 * This function makes incorrect non-null assertion fail immediately, avoiding
 * the problems with `!` at the cost of being more verbose.
 */
export function assertNonNullable<T>(value: T): NonNullable<T> {
  if (value === null || value === undefined) {
    throw new Error('value is not NonNullable');
  }
  return value;
}

/**
 * Hash the message with SHA-256 and then return the outcome in hex encoded
 * string.
 */
export async function sha256(message: string) {
  const msgBuffer = new TextEncoder().encode(message);
  const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hashHex = hashArray
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
  return hashHex;
}

/**
 * Returns a promise that resolves after the specified duration.
 */
export function timeout(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms));
}

/**
 * A utility function that helps set additional search query params.
 *
 * If the URL is a relative URL, location.origin will be used as the base.
 */
export function urlSetSearchQueryParam(
  url: string,
  key: string,
  value: { toString(): string },
) {
  const urlObj = new URL(url, location.origin);
  urlObj.searchParams.set(key, value.toString());
  return urlObj.toString();
}

/**
 * Convert a value to an Error if it's not an Error.
 */
export function toError(from: unknown): Error {
  return from instanceof Error ? from : new Error(`${from}`);
}

/**
 * Round the number up to a number in the sorted round numbers using linear
 * search.
 *
 * @param num
 * @param sortedRoundNumbers must be sorted in ascending order.
 * @return the rounded down number, or `num` if all numbers in
 *  `sortedRoundNumbers` are less than `num`.
 */
export function roundUp(num: number, sortedRoundNumbers: readonly number[]) {
  for (const predefined of sortedRoundNumbers) {
    if (num <= predefined) {
      return predefined;
    }
  }

  return num;
}

/**
 * Round the number down to a number in the sorted round numbers linear
 * search.
 *
 * @param num
 * @param sortedRoundNumbers must be sorted in ascending order.
 * @return the rounded up number, or `num` if all numbers in
 *  `sortedRoundNumbers` are greater than `num`.
 */
export function roundDown(num: number, sortedRoundNumbers: readonly number[]) {
  let lastNum = num;

  for (const predefined of sortedRoundNumbers) {
    if (num < predefined) {
      return lastNum;
    }
    lastNum = predefined;
  }

  return lastNum;
}

/**
 * Returns a promise, a resolve function to resolve the promise, and a reject
 * function to reject the promise.
 *
 * This is useful when we want to resolve/reject a promise after the
 * initialization step.
 */
export function deferred<T = void>(): [
  promise: Promise<T>,
  resolve: (value: T | PromiseLike<T>) => void,
  reject: (reason?: unknown) => void,
] {
  let resolvePromise: (value: T | PromiseLike<T>) => void;
  let rejectPromise: (reason?: unknown) => void;
  const promise = new Promise<T>((resolve, reject) => {
    resolvePromise = resolve;
    rejectPromise = reject;
  });
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
  return [promise, resolvePromise!, rejectPromise!];
}

/**
 * Converts a static string into a TrustedScriptURL if `self.trustedTypes` is
 * defined.
 *
 * @param policy the policy name to create the trusted script URL under.
 * @param staticUrl a static string that represents the URL.
 * @returns a TrustedScriptURL if `self.trustedTypes` is defined, `staticUrl`
 *  otherwise.
 */
export function createStaticTrustedURL<T extends string>(
  policy: string,
  // Force the URL to be a static string so the function can't be misused to
  // trust URLs from user input.
  staticUrl: string extends T ? never : T,
) {
  return (
    self.trustedTypes
      ?.createPolicy(policy, { createScriptURL: (_) => staticUrl })
      .createScriptURL('') || staticUrl
  );
}
