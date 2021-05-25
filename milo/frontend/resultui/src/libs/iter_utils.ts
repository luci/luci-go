// Copyright 2020 The LUCI Authors.
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
 * Yields up to certain number of items.
 */
export function* take<T>(iter: Iterable<T>, count: number): IterableIterator<T> {
  if (count <= 0) {
    return;
  }
  for (const item of iter) {
    yield item;
    count--;
    if (count === 0) {
      break;
    }
  }
}

/**
 * Yields the current item and the previous item (or null if there's no previous
 * item).
 */
export function* withPrev<T>(iter: Iterable<T>): IterableIterator<[T, T | null]> {
  let prev: T | null = null;
  for (const item of iter) {
    yield [item, prev];
    prev = item;
  }
}

/**
 * A map utility for iterators.
 */
export function* map<V, T>(iter: Iterable<V>, mapFn: (v: V) => T): IterableIterator<T> {
  for (const item of iter) {
    yield mapFn(item);
  }
}

/**
 * A map utility for async iterators.
 */
export async function* mapAsync<V, T>(iter: AsyncIterable<V>, mapFn: (v: V) => T): AsyncIterableIterator<T> {
  for await (const item of iter) {
    yield mapFn(item);
  }
}

/**
 * A flatten utility for iterators.
 */
export function* flatten<T>(iter: Iterable<readonly T[]>): IterableIterator<T> {
  for (const item of iter) {
    yield* item;
  }
}

/**
 * A utility for copying an iterator.
 * @returns a function that takes no parameter, returns a copy of the original
 * iterator
 */
export function teeAsync<T>(iter: AsyncIterator<T>): () => AsyncIterableIterator<T> {
  // TODO(weiweilin): convert cache to T[] to reduce memory usage.
  const cache = [] as Array<Promise<IteratorResult<T>> | IteratorResult<T>>;
  return async function* () {
    for (let i = 0; ; i++) {
      if (i === cache.length) {
        const next = iter.next();
        cache[i] = next;
        cache[i] = next.then((v) => (cache[i] = v));
      }
      const item = await cache[i];
      if (item.done) {
        return item.value;
      }
      yield item.value;
    }
  };
}

/**
 * Utility function that transforms the iterator to return the index of the item
 * in addition to the item.
 */
export function* enumerate<T>(iter: Iterable<T>): IterableIterator<[number, T]> {
  let i = 0;
  for (const item of iter) {
    yield [i, item];
    i++;
  }
}
