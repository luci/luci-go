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
  BatchCheckPermissionsRequest,
  BatchCheckPermissionsResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

import { Rpc } from './types';

export interface BatchedMiloInternalClientImplOpts {
  readonly service?: string;
  /**
   * Maximum number of requests in a given batch request. Defaults to 100.
   */
  readonly maxBatchSize?: number;
}

/**
 * The same as `MiloInternalClientImpl` except that eligible RPC calls are
 * batched automatically.
 */
export class BatchedMiloInternalClientImpl extends MiloInternalClientImpl {
  private readonly autoBatchedBatchCheckPermissions: (
    opt: BatchOption,
    req: BatchCheckPermissionsRequest,
  ) => Promise<BatchCheckPermissionsResponse>;

  constructor(rpc: Rpc, opts?: BatchedMiloInternalClientImplOpts) {
    super(rpc, opts);
    const maxBatchSize = opts?.maxBatchSize || 100;

    this.autoBatchedBatchCheckPermissions = batched({
      fn: (req: BatchCheckPermissionsRequest) =>
        // eslint-disable-next-line new-cap
        super.BatchCheckPermissions(req),
      shardFn: (req) => req.realm,
      combineParamSets([req1], [req2]) {
        const perms = new Set([...req1.permissions, ...req2.permissions]);
        if (perms.size > maxBatchSize) {
          return {
            ok: false,
            value: null,
          };
        }

        return {
          ok: true,
          value: [
            {
              // req1 and req2 must have the same realm because the requests are
              // sharded by realm.
              realm: req1.realm,
              permissions: [...perms.keys()],
            },
          ],
        };
      },
      splitReturn(paramSets, ret) {
        const splitRets: BatchCheckPermissionsResponse[] = [];
        for (const [req] of paramSets) {
          const results: { [key: string]: boolean } = {};
          for (const perm of req.permissions) {
            results[perm] = ret.results[perm];
          }
          splitRets.push({ results });
        }
        return splitRets;
      },
    });
  }

  BatchCheckPermissions(
    request: BatchCheckPermissionsRequest,
    opt: BatchOption = {},
  ): Promise<BatchCheckPermissionsResponse> {
    return this.autoBatchedBatchCheckPermissions(opt, request);
  }
}
