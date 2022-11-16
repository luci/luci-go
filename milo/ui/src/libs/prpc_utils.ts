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

import stableStringify from 'fast-json-stable-stringify';

export const CRITICAL_HEADERS = Object.freeze(['accept', 'content-type', 'authorization']);

/**
 * Generates a cache key for prpc-request.
 * A unique prefix should be added so different key-gen functions won't
 * accidentally generates the same key.
 *
 * Caveats:
 * * You may want to clone the request before calling this function because this
 * function consumes the request body.
 */
export async function genCacheKeyForPrpcRequest(
  prefix: string,
  req: Request,
  additionalCriticalHeaderKeys: readonly string[] = []
) {
  // We don't clone the req here so the caller can avoid cloning if they
  // don't need to reuse the request body.
  const reqBody = await req.json();

  return (
    prefix +
    stableStringify([
      req.url,
      CRITICAL_HEADERS.map((k) => req.headers.get(k)),
      additionalCriticalHeaderKeys.map((k) => req.headers.get(k)),
      // Remove properties that won't have any effect when the request hit the
      // server. e.g. `{ "pageToken": "" }` is the same as `{}`.
      removeDefaultProps(reqBody),
    ])
  );
}

/**
 * Returns a new object where false-ish properties and empty arrays from the
 * original object are removed.
 */
export function removeDefaultProps(obj: unknown): unknown {
  // Do not use `instanceof Array` because it may not work with cross frame
  // objects.
  if (Array.isArray(obj)) {
    return obj.map((ele) => removeDefaultProps(ele));
  }

  // Do not use `instanceof Object` because
  // 1. it may not work with cross frame objects, and
  // 2. functions might be classified as objects.
  // Also check `&& obj` because `typeof null === 'object'`.
  if (typeof obj === 'object' && obj) {
    return Object.fromEntries(
      Object.entries(obj).flatMap(([key, item]) => {
        if (!item || (Array.isArray(item) && item.length === 0)) {
          return [] as Array<[string, unknown]>;
        } else {
          return [[key, removeDefaultProps(item)]] as Array<[string, unknown]>;
        }
      })
    );
  }
  return obj;
}
