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
// tslint:disable-next-line: no-any
export class PeekableAsyncIterator<T, TReturn = any, TNext = undefined> implements AsyncIterator<T, TReturn, TNext>, AsyncIterable<T> {
  last?: IteratorResult<T, TReturn>;

  constructor(private iter: AsyncIterator<T>) {}

  async next() {
    if (this.last) {
      const ret = this.last;
      this.last = undefined;
      return ret;
    }

    return this.iter.next();
  }

  async peek(): Promise<IteratorResult<T, TReturn>> {
    if (!this.last) {
      this.last = await this.iter.next();
    }
    return this.last;
  }

  [Symbol.asyncIterator]() {
    return this;
  }
}
