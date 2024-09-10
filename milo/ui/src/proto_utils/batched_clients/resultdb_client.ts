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

import { BatchOption, batched } from '@/generic_libs/tools/batched_fn';
import { Sources } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import {
  BatchGetTestVariantsRequest,
  BatchGetTestVariantsResponse,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { Rpc } from '@/proto_utils/types';

const MAX_BATCH_SIZE = 100;

export interface BatchedResultDBClientImplOpts {
  readonly service?: string;
  /**
   * Maximum number of requests in a given batch request. Defaults to 100.
   */
  readonly maxBatchSize?: number;
}

/**
 * The same as `ResultDBClientImpl` except that eligible RPC calls are batched
 * automatically. Only RPC calls made via the same client instance can be
 * batched together.
 */
export class BatchedResultDBClientImpl extends ResultDBClientImpl {
  private readonly autoBatchedBatchGetTestVariants: (
    opt: BatchOption,
    req: BatchGetTestVariantsRequest,
  ) => Promise<BatchGetTestVariantsResponse>;

  constructor(rpc: Rpc, opts?: BatchedResultDBClientImplOpts) {
    super(rpc, opts);
    const maxBatchSize = opts?.maxBatchSize || MAX_BATCH_SIZE;

    this.autoBatchedBatchGetTestVariants = batched({
      fn: (req: BatchGetTestVariantsRequest) => super.BatchGetTestVariants(req),
      shardFn: (req: BatchGetTestVariantsRequest) =>
        `invocation/${req.invocation}/result-limit/${req.resultLimit}`,
      combineParamSets([req1], [req2]) {
        if (
          req1.testVariants.length + req2.testVariants.length >
          maxBatchSize
        ) {
          return {
            ok: false,
            value: null,
          };
        }

        return {
          ok: true,
          value: [
            {
              invocation: req1.invocation,
              resultLimit: req1.resultLimit,
              testVariants: [...req1.testVariants, ...req2.testVariants],
            },
          ],
        };
      },
      splitReturn(paramSets, ret) {
        let pivot = 0;
        const splitRets: BatchGetTestVariantsResponse[] = [];
        for (const [req] of paramSets) {
          const testVerdicts = ret.testVariants.slice(
            pivot,
            pivot + req.testVariants.length,
          );

          const sources: { [key: string]: Sources } = {};
          for (const tv of testVerdicts) {
            sources[tv.sourcesId] = ret.sources[tv.sourcesId];
          }

          splitRets.push({
            testVariants: testVerdicts,
            sources,
          });

          pivot += req.testVariants.length;
        }

        return splitRets;
      },
    });
  }

  BatchGetTestVariants(
    request: BatchGetTestVariantsRequest,
    opt: BatchOption = {},
  ): Promise<BatchGetTestVariantsResponse> {
    return this.autoBatchedBatchGetTestVariants(opt, request);
  }
}
