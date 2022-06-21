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

import { assert } from 'chai';
import Sinon, * as sinon from 'sinon';

import { batched, BatchOption } from './batched_fn';

interface Item {
  key: number;
  value: string;
}

describe('batched_fn', () => {
  let batchedRpc: (opt: BatchOption, ns: string, itemKeys: number[], shouldErr?: boolean) => Promise<Item[]>;
  let rpcSpy: Sinon.SinonSpy<[string, number[], boolean?], Promise<Item[]>>;
  let timer: sinon.SinonFakeTimers;

  beforeEach(() => {
    timer = sinon.useFakeTimers();
    /**
     * A function that represents an RPC.
     *
     * @param ns the namespace in which the items are defined. Requests with
     * different namespaces cannot be combined together.
     * @param itemKeys the key of the items to retrieve.
     * @param shouldErr whether the RPC will fail
     */
    function getItemsFromServer(ns: string, itemKeys: number[], shouldErr = false): Promise<Item[]> {
      if (shouldErr) {
        return Promise.reject('RPC err');
      }
      return Promise.resolve(itemKeys.map((key) => ({ key, value: `${ns}-${key}` })));
    }

    rpcSpy = sinon.spy(getItemsFromServer);
    batchedRpc = batched({
      fn: rpcSpy,
      combineParamSets: ([ns1, itemKeys1, shouldErr1], [ns2, itemKeys2, shouldErr2]) => {
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
    timer.restore();
  });

  it('should batch calls together', async () => {
    const prom1 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [1, 2]);
    const prom2 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [3, 4, 5]);
    assert.strictEqual(rpcSpy.callCount, 0);

    // Should process the pending batch at 10ms.
    assert.strictEqual(timer.runAll(), 10);
    assert.strictEqual(rpcSpy.callCount, 1);
    assert.strictEqual(rpcSpy.getCall(0).args[0], 'ns1');
    assert.deepEqual(rpcSpy.getCall(0).args[1], [1, 2, 3, 4, 5]);

    // Check returns.
    const ret1 = await prom1;
    const ret2 = await prom2;
    assert.deepEqual(ret1, [
      { key: 1, value: 'ns1-1' },
      { key: 2, value: 'ns1-2' },
    ]);
    assert.deepEqual(ret2, [
      { key: 3, value: 'ns1-3' },
      { key: 4, value: 'ns1-4' },
      { key: 5, value: 'ns1-5' },
    ]);

    // Start a new batch.
    const prom3 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [6, 7]);
    const prom4 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [8]);
    assert.strictEqual(rpcSpy.callCount, 1);

    // Should process the pending batch at 30ms.
    assert.strictEqual(timer.runAll(), 30);
    assert.strictEqual(rpcSpy.callCount, 2);
    assert.strictEqual(rpcSpy.getCall(1).args[0], 'ns1');
    assert.deepEqual(rpcSpy.getCall(1).args[1], [6, 7, 8]);

    // Check returns.
    const ret3 = await prom3;
    const ret4 = await prom4;
    assert.deepEqual(ret3, [
      { key: 6, value: 'ns1-6' },
      { key: 7, value: 'ns1-7' },
    ]);
    assert.deepEqual(ret4, [{ key: 8, value: 'ns1-8' }]);
  });

  it('should process calls within maxPendingMs', async () => {
    // Make the first call with a high maximum pending duration.
    const prom1 = batchedRpc({ maxPendingMs: 100 }, 'ns1', [1, 2]);
    assert.strictEqual(rpcSpy.callCount, 0);

    timer.tick(10);
    // The first call hasn't reached its maximum pending duration yet.
    assert.strictEqual(rpcSpy.callCount, 0);

    // Make the second call with a lower maximum pending duration.
    const prom2 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [3, 4, 5]);
    assert.strictEqual(rpcSpy.callCount, 0);

    timer.tick(10);
    // The second call reached its maximum pending duration.
    // The call should've been resolved.
    assert.strictEqual(rpcSpy.callCount, 1);
    assert.strictEqual(rpcSpy.getCall(0).args[0], 'ns1');
    assert.deepEqual(rpcSpy.getCall(0).args[1], [1, 2, 3, 4, 5]);
    const ret1 = await prom1;
    assert.deepEqual(ret1, [
      { key: 1, value: 'ns1-1' },
      { key: 2, value: 'ns1-2' },
    ]);

    // The first call should've been resolved with the second call.
    const ret2 = await prom2;
    assert.deepEqual(ret2, [
      { key: 3, value: 'ns1-3' },
      { key: 4, value: 'ns1-4' },
      { key: 5, value: 'ns1-5' },
    ]);
  });

  it('should start a new batch when the calls cannot be batched together', async () => {
    const prom1 = batchedRpc({ maxPendingMs: 100 }, 'ns1', [1, 2]);
    assert.strictEqual(rpcSpy.callCount, 0);

    timer.tick(10);
    // The first call hasn't reached its maximum pending duration yet.
    assert.strictEqual(rpcSpy.callCount, 0);

    // Schedule a second call that cannot be combined with the first call.
    const prom2 = batchedRpc({ maxPendingMs: 100 }, 'ns2', [3, 4, 5]);

    // The first call should be processed immediately, since a new batch is
    // forced to start.
    assert.strictEqual(rpcSpy.callCount, 1);
    assert.strictEqual(rpcSpy.getCall(0).args[0], 'ns1');
    assert.deepEqual(rpcSpy.getCall(0).args[1], [1, 2]);
    const ret1 = await prom1;
    assert.deepEqual(ret1, [
      { key: 1, value: 'ns1-1' },
      { key: 2, value: 'ns1-2' },
    ]);

    // Should be processed the second batch at 110ms.
    assert.strictEqual(timer.runAll(), 110);
    assert.strictEqual(rpcSpy.callCount, 2);
    assert.strictEqual(rpcSpy.getCall(1).args[0], 'ns2');
    assert.deepEqual(rpcSpy.getCall(1).args[1], [3, 4, 5]);
    const ret2 = await prom2;
    assert.deepEqual(ret2, [
      { key: 3, value: 'ns2-3' },
      { key: 4, value: 'ns2-4' },
      { key: 5, value: 'ns2-5' },
    ]);
  });

  it('should forward error to all batched calls', async () => {
    const prom1 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [1, 2]);
    const prom2 = batchedRpc({ maxPendingMs: 10 }, 'ns1', [3, 4, 5], true);
    assert.strictEqual(rpcSpy.callCount, 0);

    // Should process the pending batch at 10ms.
    assert.strictEqual(timer.runAll(), 10);
    assert.strictEqual(rpcSpy.callCount, 1);
    assert.strictEqual(rpcSpy.getCall(0).args[0], 'ns1');
    assert.deepEqual(rpcSpy.getCall(0).args[1], [1, 2, 3, 4, 5]);

    // Check returns.
    try {
      await prom1;
      assert.fail("should've thrown an error");
    } catch (e) {
      assert.equal(e, 'RPC err');
    }
    try {
      await prom2;
      assert.fail("should've thrown an err");
    } catch (e) {
      assert.equal(e, 'RPC err');
    }

    // Start a new batch.
    const prom3 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [6, 7]);
    const prom4 = batchedRpc({ maxPendingMs: 20 }, 'ns1', [8]);
    assert.strictEqual(rpcSpy.callCount, 1);

    // The new batch should work just fine.
    assert.strictEqual(timer.runAll(), 30);
    assert.strictEqual(rpcSpy.callCount, 2);
    assert.strictEqual(rpcSpy.getCall(1).args[0], 'ns1');
    assert.deepEqual(rpcSpy.getCall(1).args[1], [6, 7, 8]);

    // Check returns.
    const ret3 = await prom3;
    const ret4 = await prom4;
    assert.deepEqual(ret3, [
      { key: 6, value: 'ns1-6' },
      { key: 7, value: 'ns1-7' },
    ]);
    assert.deepEqual(ret4, [{ key: 8, value: 'ns1-8' }]);
  });
});
