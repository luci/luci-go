// Copyright 2022 The LUCI Authors.
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

import { afterEach, beforeEach, expect, jest } from '@jest/globals';

import { batched, BatchOption } from './batched_fn';

interface Item {
  key: number;
  value: string;
}

describe('batched_fn', () => {
  let batchedRpc: (
    opt: BatchOption,
    ns: string,
    itemKeys: number[],
    shouldErr?: boolean
  ) => Promise<Item[]>;
  let rpcSpy: jest.Mock<
    (ns: string, itemKeys: number[], shouldErr?: boolean) => Promise<Item[]>
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    /**
     * A function that represents an RPC.
     *
     * @param ns the namespace in which the items are defined. Requests with
     * different namespaces cannot be combined together.
     * @param itemKeys the key of the items to retrieve.
     * @param shouldErr whether the RPC will fail
     */
    function getItemsFromServer(
      ns: string,
      itemKeys: number[],
      shouldErr = false
    ): Promise<Item[]> {
      if (shouldErr) {
        return Promise.reject(new Error('RPC err'));
      }
      return Promise.resolve(
        itemKeys.map((key) => ({ key, value: `${ns}-${key}` }))
      );
    }

    rpcSpy = jest.fn(getItemsFromServer);
    batchedRpc = batched({
      fn: rpcSpy,
      combineParamSets: (
        [ns1, itemKeys1, shouldErr1],
        [ns2, itemKeys2, shouldErr2]
      ) => {
        // If we cannot combine the calls, return ResultErr.
        if (ns1 !== ns2) {
          return { ok: false, value: '' };
        }
        // If the calls can be combined, returned the combined call param set.
        return {
          ok: true,
          value: [ns1, [...itemKeys1, ...itemKeys2], shouldErr1 || shouldErr2],
        };
      },
      splitReturn: (paramSets, ret) => {
        // Split the response into multiple responses.
        // In this case, we rely on the order of the items to split the
        // response.
        // In practice, the server may use other mechanisms (e.g. request tags).
        let pivot = 0;
        const splitRets: Item[][] = [];
        for (const [, itemKeys] of paramSets) {
          splitRets.push(ret.slice(pivot, pivot + itemKeys.length));
          pivot += itemKeys.length;
        }

        return splitRets;
      },
    });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  test('should batch calls together', async () => {
    const prom1 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [1, 2]);
    const prom2 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [3, 4, 5]);
    expect(rpcSpy.mock.calls.length).toStrictEqual(0);

    // Should process the pending batch at 10ms.
    const start = jest.now();
    jest.runOnlyPendingTimers();
    expect(jest.now() - start).toStrictEqual(10);
    expect(rpcSpy.mock.calls.length).toStrictEqual(1);
    expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns1');
    expect(rpcSpy.mock.lastCall?.[1]).toEqual([1, 2, 3, 4, 5]);

    // Check returns.
    const ret1 = await prom1;
    const ret2 = await prom2;
    expect(ret1).toEqual([
      { key: 1, value: 'ns1-1' },
      { key: 2, value: 'ns1-2' },
    ]);
    expect(ret2).toEqual([
      { key: 3, value: 'ns1-3' },
      { key: 4, value: 'ns1-4' },
      { key: 5, value: 'ns1-5' },
    ]);

    // Start a new batch.
    const prom3 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [6, 7]);
    const prom4 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [8]);
    expect(rpcSpy.mock.calls.length).toStrictEqual(1);

    // Should process the pending batch at 30ms.
    jest.runOnlyPendingTimers();
    expect(jest.now() - start).toStrictEqual(30);
    expect(rpcSpy.mock.calls.length).toStrictEqual(2);
    expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns1');
    expect(rpcSpy.mock.lastCall?.[1]).toEqual([6, 7, 8]);

    // Check returns.
    const ret3 = await prom3;
    const ret4 = await prom4;
    expect(ret3).toEqual([
      { key: 6, value: 'ns1-6' },
      { key: 7, value: 'ns1-7' },
    ]);
    expect(ret4).toEqual([{ key: 8, value: 'ns1-8' }]);
  });

  test('should process calls within maxPendingMs', async () => {
    // Make the first call with a high maximum pending duration.
    const prom1 = batchedRpc({ maxPendingMs: 100 }, 'ns1', [1, 2]);
    expect(rpcSpy.mock.calls.length).toStrictEqual(0);

    jest.advanceTimersByTime(10);
    // The first call hasn't reached its maximum pending duration yet.
    expect(rpcSpy.mock.calls.length).toStrictEqual(0);

    // Make the second call with a lower maximum pending duration.
    const prom2 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [3, 4, 5]);
    expect(rpcSpy.mock.calls.length).toStrictEqual(0);

    jest.advanceTimersByTime(10);
    // The second call reached its maximum pending duration.
    // The call should've been resolved.
    expect(rpcSpy.mock.calls.length).toStrictEqual(1);
    expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns1');
    expect(rpcSpy.mock.lastCall?.[1]).toEqual([1, 2, 3, 4, 5]);
    const ret1 = await prom1;
    expect(ret1).toEqual([
      { key: 1, value: 'ns1-1' },
      { key: 2, value: 'ns1-2' },
    ]);

    // The first call should've been resolved with the second call.
    const ret2 = await prom2;
    expect(ret2).toEqual([
      { key: 3, value: 'ns1-3' },
      { key: 4, value: 'ns1-4' },
      { key: 5, value: 'ns1-5' },
    ]);
  });

  test('should start a new batch when the calls cannot be batched together', async () => {
    const prom1 = batchedRpc({ maxPendingMs: 100 }, 'ns1', [1, 2]);
    expect(rpcSpy.mock.calls.length).toStrictEqual(0);

    const start = jest.now();
    jest.advanceTimersByTime(10);
    // The first call hasn't reached its maximum pending duration yet.
    expect(rpcSpy.mock.calls.length).toStrictEqual(0);

    // Schedule a second call that cannot be combined with the first call.
    const prom2 = batchedRpc({ maxPendingMs: 100 }, 'ns2', [3, 4, 5]);

    // The first call should be processed immediately, since a new batch is
    // forced to start.
    expect(rpcSpy.mock.calls.length).toStrictEqual(1);
    expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns1');
    expect(rpcSpy.mock.lastCall?.[1]).toEqual([1, 2]);
    const ret1 = await prom1;
    expect(ret1).toEqual([
      { key: 1, value: 'ns1-1' },
      { key: 2, value: 'ns1-2' },
    ]);

    // Should be processed the second batch at 110ms.
    jest.runOnlyPendingTimers();
    expect(jest.now() - start).toStrictEqual(110);
    expect(rpcSpy.mock.calls.length).toStrictEqual(2);
    expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns2');
    expect(rpcSpy.mock.lastCall?.[1]).toEqual([3, 4, 5]);
    const ret2 = await prom2;
    expect(ret2).toEqual([
      { key: 3, value: 'ns2-3' },
      { key: 4, value: 'ns2-4' },
      { key: 5, value: 'ns2-5' },
    ]);
  });

  test('should forward error to all batched calls', async () => {
    const prom1 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [1, 2]);
    const prom2 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [3, 4, 5], true);
    expect(rpcSpy.mock.calls.length).toStrictEqual(0);

    // Should process the pending batch at 10ms.
    const start = jest.now();
    jest.runOnlyPendingTimers();
    expect(jest.now() - start).toStrictEqual(10);
    expect(rpcSpy.mock.calls.length).toStrictEqual(1);
    expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns1');
    expect(rpcSpy.mock.lastCall?.[1]).toEqual([1, 2, 3, 4, 5]);

    // Check returns.
    await expect(prom1).rejects.toThrowErrorMatchingInlineSnapshot(`"RPC err"`);
    await expect(prom2).rejects.toThrowErrorMatchingInlineSnapshot(`"RPC err"`);

    // Start a new batch.
    const prom3 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [6, 7]);
    const prom4 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [8]);
    expect(rpcSpy.mock.calls.length).toStrictEqual(1);

    // The new batch should work just fine.
    jest.runOnlyPendingTimers();
    expect(jest.now() - start).toStrictEqual(30);
    expect(rpcSpy.mock.calls.length).toStrictEqual(2);
    expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns1');
    expect(rpcSpy.mock.lastCall?.[1]).toEqual([6, 7, 8]);

    // Check returns.
    const ret3 = await prom3;
    const ret4 = await prom4;
    expect(ret3).toEqual([
      { key: 6, value: 'ns1-6' },
      { key: 7, value: 'ns1-7' },
    ]);
    expect(ret4).toEqual([{ key: 8, value: 'ns1-8' }]);
  });
});
