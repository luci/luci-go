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

export enum CacheOption {
  /**
   * Return the cached result if present.
   * Otherwise call the original function and cache the result.
   */
  Cached,
  /**
   * Refresh the cache then return the cached result.
   */
  ForceRefresh,
  /**
   * By pass the cache and call the original function.
   */
  NoCache,
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
    if (opt === CacheOption.NoCache) {
      return fn(...params);
    }
    const key = config.key(...params);
    if (opt === CacheOption.ForceRefresh || !cache.has(key)) {
      // Wraps in [] to create a unique reference.
      const value: [V] = [fn(...params)];
      const deleteCache = () => {
        // Only invalidate the cache when it has not been refreshed yet.
        if (cache.get(key) === value) {
          cache.delete(key);
        }
      };
      // Also invalidates the cache when the promise is rejected to prevent
      // unexpected cache build up.
      config.expire?.(params, value[0]).finally(deleteCache);

      // Set the cache after config.expire call so there's no memory leak when
      // config.expire throws.
      cache.set(key, value);
    }
    return cache.get(key)![0];
  };
}
