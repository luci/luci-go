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
   * Combines two parameter sets into one. If the two function calls can't be
   * combined (e.g. due to batch size limit), return a ResultErr, and the
   * pending batch will be processed immediately and a new batch will be
   * started.
   */
  readonly combineParamSets: (paramSet1: T, paramSet2: T) => Result<T, unknown>;

  /**
   * Split the return value into multiple values for each batched call.
   * The length and order of the returned batch must match that of the param
   * sets.
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
  getTimestampMs = Date.now
): (opt: BatchOption, ...params: T) => Promise<V> {
  let currentBatch: BatchState<T, V> | null = null;

  async function processCurrentBatch() {
    if (!currentBatch) {
      return;
    }

    // Move the currentBatch from the parent scope to the local scope and reset
    // the current batch.
    const batch = currentBatch;
    currentBatch = null;
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
    if (!currentBatch) {
      // No current batch. Start a new batch.
      currentBatch = {
        batchedParams: params,
        paramSets: [params],
        pendingResolves: [resolve],
        pendingRejects: [reject],
        resolveTime: Infinity,
        scheduleId: 0,
      };
    } else {
      // Attempt to combine the new call with the current batch.
      const combineResult = config.combineParamSets(
        currentBatch.batchedParams,
        params
      );
      if (combineResult.ok) {
        currentBatch.batchedParams = combineResult.value;
        currentBatch.paramSets.push(params);
        currentBatch.pendingResolves.push(resolve);
        currentBatch.pendingRejects.push(reject);
      } else {
        // The new call cannot be combined with the current batch. Process the
        // current batch immediately then start a new batch.
        processCurrentBatch();
        currentBatch = {
          batchedParams: params,
          paramSets: [params],
          pendingResolves: [resolve],
          pendingRejects: [reject],
          resolveTime: Infinity,
          scheduleId: 0,
        };
      }
    }

    const resolveTime = getTimestampMs() + (opt.maxPendingMs ?? 0);
    // If the new call needs to be resolved faster than any of the existing
    // calls, reschedule the batch to be called earlier.
    if (resolveTime < currentBatch.resolveTime) {
      currentBatch.resolveTime = resolveTime;
      clearTimeout(currentBatch.scheduleId);
      currentBatch.scheduleId = setTimeout(
        processCurrentBatch,
        opt.maxPendingMs
      );
    }

    return promise;
  };
}
