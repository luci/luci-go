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

import { batched, BatchOption } from './batched_fn';

interface Item {
  key: number;
  value: string;
}

describe('batched', () => {
  describe('without sharding', () => {
    let batchedRpc: (
      opt: BatchOption,
      ns: string,
      itemKeys: number[],
      shouldErr?: boolean,
    ) => Promise<Item[]>;
    let rpcSpy: jest.MockedFunction<
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
        shouldErr = false,
      ): Promise<Item[]> {
        if (shouldErr) {
          return Promise.reject(new Error('RPC err'));
        }
        return Promise.resolve(
          itemKeys.map((key) => ({ key, value: `${ns}-${key}` })),
        );
      }

      rpcSpy = jest.fn(getItemsFromServer);
      batchedRpc = batched({
        fn: rpcSpy,
        combineParamSets: (
          [ns1, itemKeys1, shouldErr1],
          [ns2, itemKeys2, shouldErr2],
        ) => {
          // If we cannot combine the calls, return ResultErr.
          if (ns1 !== ns2) {
            return { ok: false, value: '' };
          }
          // If the calls can be combined, returned the combined call param set.
          return {
            ok: true,
            value: [
              ns1,
              [...itemKeys1, ...itemKeys2],
              shouldErr1 || shouldErr2,
            ],
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

    it('should batch calls together', async () => {
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

    it('should process calls within maxPendingMs', async () => {
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

    it('should start a new batch when the calls cannot be batched together', async () => {
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

    it('should forward error to all batched calls', async () => {
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
      await expect(prom1).rejects.toThrowErrorMatchingInlineSnapshot(
        `"RPC err"`,
      );
      await expect(prom2).rejects.toThrowErrorMatchingInlineSnapshot(
        `"RPC err"`,
      );

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

  describe('with sharding', () => {
    let batchedRpc: (
      opt: BatchOption,
      ns: string,
      itemKeys: number[],
      shouldErr?: boolean,
    ) => Promise<Item[]>;
    let rpcSpy: jest.MockedFunction<
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
        shouldErr = false,
      ): Promise<Item[]> {
        if (shouldErr) {
          return Promise.reject(new Error(`RPC err from ${ns}`));
        }
        return Promise.resolve(
          itemKeys.map((key) => ({ key, value: `${ns}-${key}` })),
        );
      }

      rpcSpy = jest.fn(getItemsFromServer);
      batchedRpc = batched({
        fn: rpcSpy,
        shardFn: (ns) => ns,
        combineParamSets: (
          [ns1, itemKeys1, shouldErr1],
          [ns2, itemKeys2, shouldErr2],
        ) => {
          // If we cannot combine the calls, return ResultErr.
          if (ns1 !== ns2) {
            return { ok: false, value: '' };
          }
          // Cannot combine more than 10 items, return ResultErr.
          if (itemKeys1.length + itemKeys2.length > 10) {
            return { ok: false, value: '' };
          }

          // If the calls can be combined, returned the combined call param set.
          return {
            ok: true,
            value: [
              ns1,
              [...itemKeys1, ...itemKeys2],
              shouldErr1 || shouldErr2,
            ],
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

    it('should batch calls from the same shard together', async () => {
      const prom1Ns1 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [1, 2]);
      const prom1Ns2 = batchedRpc({ maxPendingMs: 10 }, 'ns2', [3, 4]);
      const prom2Ns1 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [5, 6, 7]);
      const prom2Ns2 = batchedRpc({ maxPendingMs: 10 }, 'ns2', [8, 9, 10]);
      expect(rpcSpy.mock.calls.length).toStrictEqual(0);

      // Should process the pending batch at 10ms.
      const start = jest.now();
      jest.runOnlyPendingTimers();
      expect(jest.now() - start).toStrictEqual(10);
      expect(rpcSpy.mock.calls.length).toStrictEqual(2);

      // Check returns.
      const ret1Ns1 = await prom1Ns1;
      const ret1Ns2 = await prom1Ns2;
      const ret2Ns1 = await prom2Ns1;
      const ret2Ns2 = await prom2Ns2;
      expect(ret1Ns1).toEqual([
        { key: 1, value: 'ns1-1' },
        { key: 2, value: 'ns1-2' },
      ]);
      expect(ret1Ns2).toEqual([
        { key: 3, value: 'ns2-3' },
        { key: 4, value: 'ns2-4' },
      ]);
      expect(ret2Ns1).toEqual([
        { key: 5, value: 'ns1-5' },
        { key: 6, value: 'ns1-6' },
        { key: 7, value: 'ns1-7' },
      ]);
      expect(ret2Ns2).toEqual([
        { key: 8, value: 'ns2-8' },
        { key: 9, value: 'ns2-9' },
        { key: 10, value: 'ns2-10' },
      ]);

      // Start a new batch.
      const prom3Ns1 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [11, 12]);
      const prom3Ns2 = batchedRpc({ maxPendingMs: 20 }, 'ns2', [13, 14]);
      const prom4Ns1 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [15]);
      const prom4Ns2 = batchedRpc({ maxPendingMs: 20 }, 'ns2', [16]);
      expect(rpcSpy.mock.calls.length).toStrictEqual(2);

      // Should process the pending batch at 30ms.
      jest.runOnlyPendingTimers();
      expect(jest.now() - start).toStrictEqual(30);
      expect(rpcSpy.mock.calls.length).toStrictEqual(4);

      // Check returns.
      const ret3Ns1 = await prom3Ns1;
      const ret3Ns2 = await prom3Ns2;
      const ret4Ns1 = await prom4Ns1;
      const ret4Ns2 = await prom4Ns2;
      expect(ret3Ns1).toEqual([
        { key: 11, value: 'ns1-11' },
        { key: 12, value: 'ns1-12' },
      ]);
      expect(ret3Ns2).toEqual([
        { key: 13, value: 'ns2-13' },
        { key: 14, value: 'ns2-14' },
      ]);
      expect(ret4Ns1).toEqual([{ key: 15, value: 'ns1-15' }]);
      expect(ret4Ns2).toEqual([{ key: 16, value: 'ns2-16' }]);
    });

    it('should process calls within maxPendingMs', async () => {
      // Make the 2 calls with a high maximum pending duration.
      const prom1Ns1 = batchedRpc({ maxPendingMs: 100 }, 'ns1', [1, 2]);
      const prom1Ns2 = batchedRpc({ maxPendingMs: 100 }, 'ns2', [3, 4]);
      expect(rpcSpy.mock.calls.length).toStrictEqual(0);

      jest.advanceTimersByTime(10);
      // The calls hasn't reached its maximum pending duration yet.
      expect(rpcSpy.mock.calls.length).toStrictEqual(0);

      // Make another call with a lower maximum pending duration for one
      // namespace.
      const prom2Ns1 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [5, 6, 7]);
      expect(rpcSpy.mock.calls.length).toStrictEqual(0);

      jest.advanceTimersByTime(10);
      // The second call for ns1 reached its maximum pending duration.
      // The call should've been resolved.
      expect(rpcSpy.mock.calls.length).toStrictEqual(1);
      expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns1');
      expect(rpcSpy.mock.lastCall?.[1]).toEqual([1, 2, 5, 6, 7]);
      const ret2Ns1 = await prom2Ns1;
      expect(ret2Ns1).toEqual([
        { key: 5, value: 'ns1-5' },
        { key: 6, value: 'ns1-6' },
        { key: 7, value: 'ns1-7' },
      ]);

      // The first call should've been resolved with the second call.
      const ret1Ns1 = await prom1Ns1;
      expect(ret1Ns1).toEqual([
        { key: 1, value: 'ns1-1' },
        { key: 2, value: 'ns1-2' },
      ]);

      jest.advanceTimersByTime(80);
      // The call for ns2 will still be processed at 100ms since its pending
      // duration is not affected by another call.
      expect(rpcSpy.mock.calls.length).toStrictEqual(2);
      expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns2');
      expect(rpcSpy.mock.lastCall?.[1]).toEqual([3, 4]);
      const ret1Ns2 = await prom1Ns2;
      expect(ret1Ns2).toEqual([
        { key: 3, value: 'ns2-3' },
        { key: 4, value: 'ns2-4' },
      ]);
    });

    it('should start a new batch when the calls cannot be batched together in a shard', async () => {
      const prom1Ns1 = batchedRpc(
        { maxPendingMs: 100 },
        'ns1',
        [1, 2, 3, 4, 5, 6],
      );
      const prom1Ns2 = batchedRpc(
        { maxPendingMs: 100 },
        'ns2',
        [2, 3, 4, 5, 6, 7],
      );
      expect(rpcSpy.mock.calls.length).toStrictEqual(0);

      const start = jest.now();
      jest.advanceTimersByTime(10);
      // The first call hasn't reached its maximum pending duration yet.
      expect(rpcSpy.mock.calls.length).toStrictEqual(0);

      // Schedule a second call that cannot be combined with the first call in
      // ns1.
      const prom2Ns1 = batchedRpc(
        { maxPendingMs: 100 },
        'ns1',
        [7, 8, 9, 10, 11, 12],
      );
      // Schedule a second call that can be combined with the first call in
      // ns2.
      const prom2Ns2 = batchedRpc({ maxPendingMs: 100 }, 'ns2', [8, 9]);

      // The first call should be processed immediately, since a new batch is
      // forced to start.
      expect(rpcSpy.mock.calls.length).toStrictEqual(1);
      expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns1');
      expect(rpcSpy.mock.lastCall?.[1]).toEqual([1, 2, 3, 4, 5, 6]);
      const ret1Ns1 = await prom1Ns1;
      expect(ret1Ns1).toEqual([
        { key: 1, value: 'ns1-1' },
        { key: 2, value: 'ns1-2' },
        { key: 3, value: 'ns1-3' },
        { key: 4, value: 'ns1-4' },
        { key: 5, value: 'ns1-5' },
        { key: 6, value: 'ns1-6' },
      ]);

      // The requests in ns2 can still be batched as usual.
      jest.advanceTimersByTime(90);
      expect(rpcSpy.mock.calls.length).toStrictEqual(2);
      expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns2');
      expect(rpcSpy.mock.lastCall?.[1]).toEqual([2, 3, 4, 5, 6, 7, 8, 9]);
      const ret1Ns2 = await prom1Ns2;
      expect(ret1Ns2).toEqual([
        { key: 2, value: 'ns2-2' },
        { key: 3, value: 'ns2-3' },
        { key: 4, value: 'ns2-4' },
        { key: 5, value: 'ns2-5' },
        { key: 6, value: 'ns2-6' },
        { key: 7, value: 'ns2-7' },
      ]);
      const ret2Ns2 = await prom2Ns2;
      expect(ret2Ns2).toEqual([
        { key: 8, value: 'ns2-8' },
        { key: 9, value: 'ns2-9' },
      ]);

      // Should be processed the second batch in ns1 at 110ms.
      jest.runOnlyPendingTimers();
      expect(jest.now() - start).toStrictEqual(110);
      expect(rpcSpy.mock.calls.length).toStrictEqual(3);
      expect(rpcSpy.mock.lastCall?.[0]).toStrictEqual('ns1');
      expect(rpcSpy.mock.lastCall?.[1]).toEqual([7, 8, 9, 10, 11, 12]);
      const ret2Ns1 = await prom2Ns1;
      expect(ret2Ns1).toEqual([
        { key: 7, value: 'ns1-7' },
        { key: 8, value: 'ns1-8' },
        { key: 9, value: 'ns1-9' },
        { key: 10, value: 'ns1-10' },
        { key: 11, value: 'ns1-11' },
        { key: 12, value: 'ns1-12' },
      ]);
    });

    it('should forward error to all batched calls in the same shard', async () => {
      const prom1Ns1 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [1, 2]);
      const prom1Ns2 = batchedRpc({ maxPendingMs: 10 }, 'ns2', [2, 3]);
      const prom1Ns3 = batchedRpc({ maxPendingMs: 10 }, 'ns3', [3, 4]);
      const prom2Ns1 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [4, 5, 6], true);
      const prom2Ns2 = batchedRpc({ maxPendingMs: 10 }, 'ns2', [5, 6, 7], true);
      const prom2Ns3 = batchedRpc({ maxPendingMs: 10 }, 'ns3', [6, 7, 8]);
      expect(rpcSpy.mock.calls.length).toStrictEqual(0);

      // Should process the pending batch at 10ms.
      const start = jest.now();
      jest.runOnlyPendingTimers();
      expect(jest.now() - start).toStrictEqual(10);

      // Check returns.
      expect(rpcSpy.mock.calls.length).toStrictEqual(3);
      await expect(prom1Ns1).rejects.toThrowErrorMatchingInlineSnapshot(
        `"RPC err from ns1"`,
      );
      await expect(prom2Ns1).rejects.toThrowErrorMatchingInlineSnapshot(
        `"RPC err from ns1"`,
      );
      // Responses from another shard have different errors.
      await expect(prom1Ns2).rejects.toThrowErrorMatchingInlineSnapshot(
        `"RPC err from ns2"`,
      );
      await expect(prom2Ns2).rejects.toThrowErrorMatchingInlineSnapshot(
        `"RPC err from ns2"`,
      );
      // Responses from a successful shard are not affected.
      const ret1Ns3 = await prom1Ns3;
      const ret2Ns3 = await prom2Ns3;
      expect(ret1Ns3).toEqual([
        { key: 3, value: 'ns3-3' },
        { key: 4, value: 'ns3-4' },
      ]);
      expect(ret2Ns3).toEqual([
        { key: 6, value: 'ns3-6' },
        { key: 7, value: 'ns3-7' },
        { key: 8, value: 'ns3-8' },
      ]);

      // Start a new batch.
      const prom3Ns1 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [1]);
      const prom3Ns2 = batchedRpc({ maxPendingMs: 20 }, 'ns2', [2]);
      const prom3Ns3 = batchedRpc({ maxPendingMs: 20 }, 'ns3', [3]);
      const prom4Ns1 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [4, 5]);
      const prom4Ns2 = batchedRpc({ maxPendingMs: 20 }, 'ns2', [5, 6]);
      const prom4Ns3 = batchedRpc({ maxPendingMs: 20 }, 'ns3', [6, 7]);
      expect(rpcSpy.mock.calls.length).toStrictEqual(3);

      // The new batch should work just fine.
      jest.runOnlyPendingTimers();
      expect(jest.now() - start).toStrictEqual(30);
      expect(rpcSpy.mock.calls.length).toStrictEqual(6);

      // Check returns.
      const ret3Ns1 = await prom3Ns1;
      const ret3Ns2 = await prom3Ns2;
      const ret3Ns3 = await prom3Ns3;
      const ret4Ns1 = await prom4Ns1;
      const ret4Ns2 = await prom4Ns2;
      const ret4Ns3 = await prom4Ns3;
      expect(ret3Ns1).toEqual([{ key: 1, value: 'ns1-1' }]);
      expect(ret3Ns2).toEqual([{ key: 2, value: 'ns2-2' }]);
      expect(ret3Ns3).toEqual([{ key: 3, value: 'ns3-3' }]);
      expect(ret4Ns1).toEqual([
        { key: 4, value: 'ns1-4' },
        { key: 5, value: 'ns1-5' },
      ]);
      expect(ret4Ns2).toEqual([
        { key: 5, value: 'ns2-5' },
        { key: 6, value: 'ns2-6' },
      ]);
      expect(ret4Ns3).toEqual([
        { key: 6, value: 'ns3-6' },
        { key: 7, value: 'ns3-7' },
      ]);
    });
  });
});
