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

export interface CacheConfig<T extends unknown[]> {
  /**
   * Computes a cache key given the parameters.
   */
  key: (...params: T) => unknown;
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
  config: CacheConfig<T>,
): (opt: CacheOption, ...params: T) => V {
  const cache = new Map<unknown, V>();

  return (opt: CacheOption, ...params: T) => {
    if (opt === CacheOption.NoCache) {
      return fn(...params);
    }
    const key = config.key(...params);
    if (opt === CacheOption.ForceRefresh || !cache.has(key)) {
      cache.set(key, fn(...params));
    }
    return cache.get(key) as V;
  };
}
