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

import { get as kvGet, set as kvSet } from 'idb-keyval';
import { createHandlerBoundToURL as workboxCreateHandlerBoundToURL } from 'workbox-precaching';

// Tell TSC that this is a ServiceWorker script.
declare const self: ServiceWorkerGlobalScope;

const LAST_KNOWN_FRESH_TIME_KEY = 'last-known-fresh-time-54c0ca82';

/**
 * Update the time the service worker is last known to be fresh (i.e. no new
 * version is found).
 *
 * EXPORTED FOR TEST PURPOSE ONLY.
 */
export async function _updateLastKnownFreshTime() {
  await kvSet(LAST_KNOWN_FRESH_TIME_KEY, Date.now());
}

self.registration
  ?.update() // Add `?` to ensure this works in unit tests.
  .finally(() => kvSet(LAST_KNOWN_FRESH_TIME_KEY, Date.now()));

export interface CreateHandlerBoundToURLOptions {
  /**
   * Stop serving cache to after `staleWhileRevalidate` since the service worker
   * is last known to be fresh.
   *
   * Use `updateKnownFreshTime()` to update the last known fresh time of the
   * service worker.
   */
  readonly staleWhileRevalidate: number;
}

/**
 * Similar to `createHandlerBoundToURL` from `workbox-precaching` except that it
 * supports `stale-while-revalidate` logic.
 */
export function createHandlerBoundToURL(
  url: string,
  opts: CreateHandlerBoundToURLOptions,
) {
  const handlerBound = workboxCreateHandlerBoundToURL(url);

  let shouldRevalidate = true;
  return async (options: Parameters<typeof handlerBound>[0]) => {
    // Only update the cached shouldRevalidate value if it's true.
    // We only want to bypass the cache when the service worker has not been
    // used for a long time (e.g. due to user inactivity). If the service worker
    // is alive in memory, then the service worker must be relatively fresh.
    // There's no need to check IndexDB again to determine the last update time.
    if (shouldRevalidate) {
      const lastKnownFreshTime = await kvGet<number>(LAST_KNOWN_FRESH_TIME_KEY);
      shouldRevalidate =
        lastKnownFreshTime === undefined ||
        lastKnownFreshTime + opts.staleWhileRevalidate < Date.now();
    }

    if (shouldRevalidate) {
      return fetch('/ui/index.html');
    }
    return handlerBound(options);
  };
}
