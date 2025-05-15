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

import stableStringify from 'fast-json-stable-stringify';
import { useContext, useEffect, useRef } from 'react';

import { SingletonStoreCtx } from './context';

export interface UseSingletonOptions<T> {
  /**
   * The key used to retrieve a previously constructed `T`.
   *
   * Similar to react-query's queryKey, this key will be hashed into a stable
   * hash.
   */
  readonly key: readonly unknown[];
  /**
   * The function that constructs a `T`.
   */
  readonly fn: () => T;
}

/**
 * `useSingleton` enables the use of singleton pattern in React.
 *
 * A memorized instance of `T` will be returned if there's a memorized `T`
 * with the same key in a `<SingletonStoreProvider />`. Otherwise, a new
 * instance of `T` will be created from the provided function, and the instance
 * will be memorized until there's no component in the same
 * `<SingletonStoreProvider />` calling `useSingleton` with the same key.
 *
 * It's similar to `useQuery` from `react-query` except that
 * 1. it's synchronized (i.e. no state change), and
 * 2. the value will be cached if and only if there's an active observer for the
 *    same key.
 */
export function useSingleton<T>({ key, fn }: UseSingletonOptions<T>): T {
  const store = useContext(SingletonStoreCtx);
  if (store === undefined) {
    throw new Error(
      'useSingleton can only be used in a SingletonStoreProvider',
    );
  }

  const hookId = useRef(undefined);
  const strKey = stableStringify(key);
  const handle = store.getOrCreate(hookId, strKey, fn);

  useEffect(() => {
    // In practice, we don't need to subscribe here because we already
    // subscribed during render. Still subscribe anyway so the clean up function
    // is the exact reverse of the `useEffect` callback. This ensures the hook
    // works in React strict mode in a dev build.
    handle.subscribe(hookId);
    return () => handle.unsubscribe(hookId);
  }, [handle]);

  return handle.getValue() as T;
}
