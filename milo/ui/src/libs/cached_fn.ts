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

export interface CacheConfig<T extends unknown[], V> {
  /**
   * Computes a cache key given the parameters.
   */
  key: (...params: T) => unknown;

  /**
   * When the promise resolves or rejects, the cache is invalidated.
   *
   * When not specified, caches are kept indefinitely.
   */
  // Return a promise instead of a simple number so the function can await on
  // result if the cache duration depends on it.
  expire?: (params: T, result: V) => Promise<void>;
}

export interface CacheOption {
  /**
   * Whether the cache result should be returned on a cache hit.
   *
   * Default to true.
   */
  acceptCache?: boolean;
  /**
   * Whether cache update should be skipped when a new result is computed.
   * Cache update is always skipped if no new results were computed (e.g. on a
   * cache hit) regardless of this setting.
   *
   * Default to false.
   */
  skipUpdate?: boolean;
  /**
   * Whether the existing cache should be invalidated.
   * If acceptCache is set to true, the existing cache will still be returned
   * before being invalidated. This is useful when the cached value can't be
   * reused (e.g. Response.body).
   *
   * Default to false.
   */
  invalidateCache?: boolean;
}

/**
 * Construct a function that returns cache result when called with the same
 * parameters.
 */
export function cached<T extends unknown[], V>(
  fn: (...params: T) => V,
  config: CacheConfig<T, V>
): (opt: CacheOption, ...params: T) => V {
  const cache = new Map<unknown, [V]>();

  return (opt: CacheOption, ...params: T) => {
    const key = config.key(...params);
    let value: V;
    let updatedCache = false;

    if ((opt.acceptCache ?? true) && cache.has(key)) {
      value = cache.get(key)![0];
    } else {
      value = fn(...params);
      if (!opt.skipUpdate) {
        // Wraps in [] to create a unique reference.
        const valueRef: [V] = [value];
        const deleteCache = () => {
          // Only invalidate the cache when it has not been refreshed yet.
          if (cache.get(key) === valueRef) {
            cache.delete(key);
          }
        };
        // Also invalidates the cache when the promise is rejected to prevent
        // unexpected cache build up.
        config
          .expire?.(params, value)
          .catch((_e) => {})
          .finally(deleteCache);

        // Set the cache after config.expire call so there's no memory leak when
        // config.expire throws.
        cache.set(key, valueRef);
        updatedCache = true;
      }
    }

    // Only invalidates old cache values.
    if (!updatedCache && opt.invalidateCache) {
      cache.delete(key);
    }

    return value;
  };
}
