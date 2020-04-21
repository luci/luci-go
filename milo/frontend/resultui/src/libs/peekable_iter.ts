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
 * Extends an async iterator with peek method.
 */
export class PeekableAsyncIterator<T, TReturn = unknown, TNext = undefined> implements AsyncIterator<T, TReturn, TNext>, AsyncIterable<T> {
  private last?: Promise<IteratorResult<T, TReturn>>;

  constructor(private iter: AsyncIterator<T>) {}

  next() {
    if (this.last) {
      const ret = this.last;
      this.last = undefined;
      return ret;
    }

    return this.iter.next();
  }

  /**
   * Get the next item without removing it from this iterator.
   *
   * Note that the item is still removed from the iterator passed to the
   * constructor.
   */
  peek(): Promise<IteratorResult<T, TReturn>> {
    if (!this.last) {
      this.last = this.iter.next();
    }
    return this.last;
  }

  [Symbol.asyncIterator]() {
    return this;
  }
}
