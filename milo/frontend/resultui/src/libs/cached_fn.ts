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
   * By default, JSON.stringify(params) is used.
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

export interface CachedFn<T extends unknown[], V> {
  (...params: T): V;
  withOpt: (opt: CacheOption, ...params: T) => V;
}

export function cached<T extends unknown[], V>(
  fn: (...params: T) => V,
  config: CacheConfig<T>,
): CachedFn<T, V> {
  const cache = new Map<unknown, V>();

  const cachedFn = (...params: T) => {
    const key = config.key(...params);
    if (!cache.has(key)) {
      cache.set(key, fn(...params));
    }
    return cache.get(key) as V;
  };

  cachedFn.withOpt = (opt: CacheOption, ...params: T) => {
    switch (opt) {
      case CacheOption.Cached:
        return cachedFn(...params);
      case CacheOption.ForceRefresh: {
        const key = config.key(...params);
        const ret = fn(...params);
        cache.set(key, ret);
        return ret;
      }
      case CacheOption.NoCache:
        return fn(...params);
      default:
        throw new Error(`invalid cache option ${opt}`);
    }
  };

  return cachedFn;
}
