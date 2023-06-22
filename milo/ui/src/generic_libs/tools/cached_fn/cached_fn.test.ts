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

import { beforeEach, expect, jest } from '@jest/globals';
import stableStringify from 'fast-json-stable-stringify';

import { timeout } from '@/generic_libs/tools/utils';

import { cached, CacheOption } from './cached_fn';

describe('cached_fn', () => {
  let cachedFn: (opt: CacheOption, param1: number, param2: string) => string;
  let fnSpy: jest.Mock<(param1: number, param2: string) => string>;

  beforeEach(() => {
    jest.useFakeTimers();
    let callCount = 0;
    const fn = (param1: number, param2: string) =>
      `${param1}-${param2}-${callCount++}`;
    fnSpy = jest.fn(fn);
    cachedFn = cached(fnSpy, { key: (...params) => stableStringify(params) });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  test('should return cached response when params are identical', async () => {
    const res1 = cachedFn({}, 1, 'a');
    const res2 = cachedFn({}, 1, 'a');
    expect(res1).toStrictEqual(res2);
    expect(fnSpy.mock.calls.length).toStrictEqual(1);
  });

  test('should return cached response when params are different', async () => {
    const res1 = cachedFn({}, 1, 'a');
    const res2 = cachedFn({}, 2, 'a');
    const res3 = cachedFn({}, 1, 'b');
    expect(res1).toStrictEqual('1-a-0');
    expect(res2).toStrictEqual('2-a-1');
    expect(res3).toStrictEqual('1-b-2');
    expect(fnSpy.mock.calls.length).toStrictEqual(3);
  });

  test('should be able to cache multiple different function calls', async () => {
    const res1a = cachedFn({}, 1, 'a');
    const res2a = cachedFn({}, 2, 'a');
    const res3a = cachedFn({}, 1, 'b');
    const res1b = cachedFn({}, 1, 'a');
    const res2b = cachedFn({}, 2, 'a');
    const res3b = cachedFn({}, 1, 'b');
    expect(res1a).toStrictEqual(res1b);
    expect(res2a).toStrictEqual(res2b);
    expect(res3a).toStrictEqual(res3b);
    expect(fnSpy.mock.calls.length).toStrictEqual(3);
  });

  test('should refresh the cache when acceptCache = false', async () => {
    const res1 = cachedFn({}, 1, 'a');
    const res2 = cachedFn({ acceptCache: false }, 1, 'a');
    const res3 = cachedFn({}, 1, 'a');
    expect(res1).toStrictEqual('1-a-0');
    expect(res2).toStrictEqual('1-a-1');
    expect(res3).toStrictEqual('1-a-1');
    expect(fnSpy.mock.calls.length).toStrictEqual(2);
  });

  test('should not update the cache when calling with skipUpdate = true', async () => {
    const res1 = cachedFn({}, 1, 'a');
    const res2 = cachedFn({ acceptCache: false, skipUpdate: true }, 1, 'a');
    const res3 = cachedFn({}, 1, 'a');
    expect(res1).toStrictEqual('1-a-0');
    expect(res2).toStrictEqual('1-a-1');
    expect(res3).toStrictEqual('1-a-0');
    expect(fnSpy.mock.calls.length).toStrictEqual(2);
  });

  test('should invalidate the old cache when invalidateCache = true', async () => {
    const res1 = cachedFn({}, 1, 'a');
    const res2 = cachedFn({ invalidateCache: true }, 1, 'a');
    const res3 = cachedFn({}, 1, 'a');
    expect(res1).toStrictEqual('1-a-0');
    expect(res2).toStrictEqual('1-a-0');
    expect(res3).toStrictEqual('1-a-1');
    expect(fnSpy.mock.calls.length).toStrictEqual(2);
  });

  test('should not invalidate the new cache when invalidateCache = true', async () => {
    const res1 = cachedFn({}, 1, 'a');
    const res2 = cachedFn(
      { acceptCache: false, invalidateCache: true },
      1,
      'a'
    );
    const res3 = cachedFn({}, 1, 'a');
    expect(res1).toStrictEqual('1-a-0');
    expect(res2).toStrictEqual('1-a-1');
    expect(res3).toStrictEqual('1-a-1');
    expect(fnSpy.mock.calls.length).toStrictEqual(2);
  });

  describe('when config.expire(...) returns a promise that resolves', () => {
    beforeEach(() => {
      cachedFn = cached(fnSpy, {
        key: (...params) => stableStringify(params),
        expire: () => timeout(20),
      });
    });

    test('should return cached response when cache has not expired', async () => {
      const res1 = cachedFn({}, 1, 'a');
      await jest.advanceTimersByTimeAsync(10);
      const res2 = cachedFn({}, 1, 'a');
      expect(res1).toStrictEqual(res2);
      expect(fnSpy.mock.calls.length).toStrictEqual(1);
    });

    test('should return a new response when cache has expired', async () => {
      const res1 = cachedFn({}, 1, 'a');
      await jest.advanceTimersByTimeAsync(30);
      const res2 = cachedFn({}, 1, 'a');
      expect(res1).toStrictEqual('1-a-0');
      expect(res2).toStrictEqual('1-a-1');
      expect(fnSpy.mock.calls.length).toStrictEqual(2);
    });

    test('should not expire refreshed cache too early', async () => {
      const res1 = cachedFn({}, 1, 'a');
      await jest.advanceTimersByTimeAsync(15);
      const res2 = cachedFn({ acceptCache: false }, 1, 'a');
      await jest.advanceTimersByTimeAsync(15);
      const res3 = cachedFn({}, 1, 'a');
      expect(res1).toStrictEqual('1-a-0');
      expect(res2).toStrictEqual('1-a-1');
      expect(res3).toStrictEqual('1-a-1');
      expect(fnSpy.mock.calls.length).toStrictEqual(2);
    });
  });

  describe('when config.expire() returns a promise that rejects', () => {
    beforeEach(() => {
      cachedFn = cached(fnSpy, {
        key: (...params) => stableStringify(params),
        expire: async () => {
          await timeout(20);
          throw new Error();
        },
      });
    });

    test('should return cached response when cache has not expired', async () => {
      const res1 = cachedFn({}, 1, 'a');
      await jest.advanceTimersByTimeAsync(10);
      const res2 = cachedFn({}, 1, 'a');
      expect(res1).toStrictEqual(res2);
      expect(fnSpy.mock.calls.length).toStrictEqual(1);
    });

    test('should return a new response when cache has expired', async () => {
      const res1 = cachedFn({}, 1, 'a');
      await jest.advanceTimersByTimeAsync(30);
      const res2 = cachedFn({}, 1, 'a');
      expect(res1).toStrictEqual('1-a-0');
      expect(res2).toStrictEqual('1-a-1');
      expect(fnSpy.mock.calls.length).toStrictEqual(2);
    });

    test('should not invalidate refreshed cache too early', async () => {
      const res1 = cachedFn({}, 1, 'a');
      await jest.advanceTimersByTimeAsync(15);
      const res2 = cachedFn({ acceptCache: false }, 1, 'a');
      await jest.advanceTimersByTimeAsync(15);
      const res3 = cachedFn({}, 1, 'a');
      expect(res1).toStrictEqual('1-a-0');
      expect(res2).toStrictEqual('1-a-1');
      expect(res3).toStrictEqual('1-a-1');
      expect(fnSpy.mock.calls.length).toStrictEqual(2);
    });
  });

  describe('when config.expire() resolves immediately', () => {
    beforeEach(() => {
      cachedFn = cached(fnSpy, {
        key: (...params) => stableStringify(params),
        expire: () => Promise.resolve(),
      });
    });

    test('should not delete the cache before the function returns', async () => {
      const res1 = cachedFn({}, 1, 'a');
      expect(res1).toStrictEqual('1-a-0');
      expect(fnSpy.mock.calls.length).toStrictEqual(1);
    });

    test('should delete the cache in the next event cycle', async () => {
      const res1 = cachedFn({}, 1, 'a');
      await jest.advanceTimersByTimeAsync(0);
      const res2 = cachedFn({}, 1, 'a');
      expect(res1).toStrictEqual('1-a-0');
      expect(res2).toStrictEqual('1-a-1');
      expect(fnSpy.mock.calls.length).toStrictEqual(2);
    });
  });

  describe('when config.expire() throws immediately', () => {
    beforeEach(() => {
      let firstCall = true;
      cachedFn = cached(fnSpy, {
        key: (...params) => stableStringify(params),
        expire: () => {
          if (firstCall) {
            firstCall = false;
            throw new Error();
          }
          return Promise.resolve();
        },
      });
    });

    test('should not cache the response', async () => {
      expect(() => cachedFn({}, 1, 'a')).toThrow();
      const res2 = cachedFn({}, 1, 'a');
      expect(res2).toStrictEqual('1-a-1');
      expect(fnSpy.mock.calls.length).toStrictEqual(2);
    });
  });
});
