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

import { deferred } from '@/generic_libs/tools/utils';

export interface BatchConfig<T extends unknown[], V> {
  /**
   * The function to be batched.
   */
  readonly fn: (...params: T) => Promise<V>;

  /**
   * When specified, only requests in the same shard can be batched together.
   */
  readonly shardFn?: (...param: T) => string;

  /**
   * Combines two parameter sets into one. If the two function calls can't be
   * combined (e.g. due to batch size limit), return a ResultErr, and the
   * pending batch will be processed immediately and a new batch will be
   * started.
   *
   * If `shardFn` is specified, `paramSet1` and `paramSet2` are guaranteed to be
   * in the same shard.
   */
  readonly combineParamSets: (paramSet1: T, paramSet2: T) => Result<T, unknown>;

  /**
   * Split the return value into multiple values for each batched call.
   * The length and order of the returned batch must match that of the param
   * sets.
   *
   * If `shardFn` is specified, `paramSets` are guaranteed to be in the same
   * shard.
   */
  readonly splitReturn: (paramSets: readonly T[], ret: V) => readonly V[];
}

export interface BatchOption {
  /**
   * The maximum pending duration for this call. Default to 0.
   *
   * The batch will be processed when any of the calls reaches its maximum
   * pending duration.
   */
  maxPendingMs?: number;
}

interface BatchState<T extends unknown[], V> {
  batchedParams: T;
  readonly paramSets: T[];
  readonly pendingResolves: Array<(ret: V) => void>;
  readonly pendingRejects: Array<(reason?: unknown) => void>;
  resolveTime: number;
  scheduleId: number;
}

/**
 * Construct a function that can batches multiple function calls together. This
 * is useful for batching multiple RPC calls together.
 *
 * Check the test file for example.
 */
export function batched<T extends unknown[], V>(
  config: BatchConfig<T, V>,
  setTimeout = self.setTimeout,
  clearTimeout = self.clearTimeout,
  getTimestampMs = Date.now,
): (opt: BatchOption, ...params: T) => Promise<V> {
  const shardCurrentBatchMap = new Map<string, BatchState<T, V>>();

  async function processCurrentBatch(shard: string) {
    const batch = shardCurrentBatchMap.get(shard);
    if (!batch) {
      return;
    }

    shardCurrentBatchMap.delete(shard);
    clearTimeout(batch.scheduleId);

    try {
      const batchedRet = await config.fn(...batch.batchedParams);
      const rets = config.splitReturn(batch.paramSets, batchedRet);
      for (let i = 0; i < batch.pendingResolves.length; ++i) {
        batch.pendingResolves[i](rets[i]);
      }
    } catch (e) {
      for (const reject of batch.pendingRejects) {
        reject(e);
      }
    }
  }

  return async (opt, ...params) => {
    const [promise, resolve, reject] = deferred<V>();
    const shard = config.shardFn?.(...params) || '';
    let currentBatch = shardCurrentBatchMap.get(shard);
    if (!currentBatch) {
      // No current batch in this shard. Start a new batch.
      currentBatch = {
        batchedParams: params,
        paramSets: [params],
        pendingResolves: [resolve],
        pendingRejects: [reject],
        resolveTime: Infinity,
        scheduleId: 0,
      };
      shardCurrentBatchMap.set(shard, currentBatch);
    } else {
      // Attempt to combine the new call with the current batch.
      const combineResult = config.combineParamSets(
        currentBatch.batchedParams,
        params,
      );
      if (combineResult.ok) {
        currentBatch.batchedParams = combineResult.value;
        currentBatch.paramSets.push(params);
        currentBatch.pendingResolves.push(resolve);
        currentBatch.pendingRejects.push(reject);
      } else {
        // The new call cannot be combined with the current batch. Process the
        // current batch immediately then start a new batch.
        processCurrentBatch(shard);
        currentBatch = {
          batchedParams: params,
          paramSets: [params],
          pendingResolves: [resolve],
          pendingRejects: [reject],
          resolveTime: Infinity,
          scheduleId: 0,
        };
        shardCurrentBatchMap.set(shard, currentBatch);
      }
    }

    const resolveTime = getTimestampMs() + (opt.maxPendingMs ?? 0);
    // If the new call needs to be resolved faster than any of the existing
    // calls, reschedule the batch to be called earlier.
    if (resolveTime < currentBatch.resolveTime) {
      currentBatch.resolveTime = resolveTime;
      clearTimeout(currentBatch.scheduleId);
      currentBatch.scheduleId = setTimeout(
        () => processCurrentBatch(shard),
        opt.maxPendingMs,
      );
    }

    return promise;
  };
}
