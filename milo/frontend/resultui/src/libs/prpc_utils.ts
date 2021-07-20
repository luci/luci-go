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
 * Removes false-ish properties and empty arrays from the object.
 *
 * Returns the same object instance for convenience.
 */
export function removeDefaultProps(obj: unknown): unknown {
  if (obj instanceof Array) {
    for (const item of obj) {
      removeDefaultProps(item);
    }
  } else if (obj instanceof Object) {
    for (const [key, item] of Object.entries(obj)) {
      if (!item || (item instanceof Array && item.length === 0)) {
        delete (obj as { [key: string]: unknown })[key];
      } else {
        removeDefaultProps(item);
      }
    }
  }
  return obj;
}
