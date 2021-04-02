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
  return (
    prefix +
    JSON.stringify([
      req.url,
      CRITICAL_HEADERS.map((k) => req.headers.get(k)),
      additionalCriticalHeaderKeys.map((k) => req.headers.get(k)),
      // We don't clone the req here so the caller can avoid cloning if they
      // don't need to reuse the request body.
      await req.text(),
    ])
  );
}
