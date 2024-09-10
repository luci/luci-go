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
import {
  BatchGetTestVariantBranchRequest,
  BatchGetTestVariantBranchResponse,
  TestVariantBranchesClientImpl,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { Rpc } from '@/proto_utils/types';

const MAX_BATCH_SIZE = 100;

export interface BatchedTestVariantBranchesClientImplOpts {
  readonly service?: string;
  /**
   * Maximum number of names in a given `BatchGet` request. Defaults to 100.
   */
  readonly maxBatchGetSize?: number;
}

/**
 * The same as `TestVariantBranchesClientImpl` except that eligible RPC calls
 * are batched automatically. Only RPC calls made via the same client instance
 * can be batched together.
 */
export class BatchedTestVariantBranchesClientImpl extends TestVariantBranchesClientImpl {
  private readonly autoBatchedBatchGet: (
    opt: BatchOption,
    req: BatchGetTestVariantBranchRequest,
  ) => Promise<BatchGetTestVariantBranchResponse>;

  constructor(rpc: Rpc, opts?: BatchedTestVariantBranchesClientImplOpts) {
    super(rpc, opts);
    const maxBatchGetSize = opts?.maxBatchGetSize || MAX_BATCH_SIZE;

    this.autoBatchedBatchGet = batched({
      // eslint-disable-next-line new-cap
      fn: (req: BatchGetTestVariantBranchRequest) => super.BatchGet(req),
      combineParamSets([req1], [req2]) {
        if (req1.names.length + req2.names.length > maxBatchGetSize) {
          return {
            ok: false,
            value: null,
          };
        }

        return {
          ok: true,
          value: [
            {
              names: [...req1.names, ...req2.names],
            },
          ],
        };
      },
      splitReturn(paramSets, ret) {
        let pivot = 0;
        const splitRets: BatchGetTestVariantBranchResponse[] = [];
        for (const [req] of paramSets) {
          splitRets.push({
            testVariantBranches: ret.testVariantBranches.slice(
              pivot,
              pivot + req.names.length,
            ),
          });
          pivot += req.names.length;
        }

        return splitRets;
      },
    });
  }

  BatchGet(
    request: BatchGetTestVariantBranchRequest,
    opt: BatchOption = {},
  ): Promise<BatchGetTestVariantBranchResponse> {
    return this.autoBatchedBatchGet(opt, request);
  }
}
