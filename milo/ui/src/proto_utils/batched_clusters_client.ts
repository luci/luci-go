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
  ClusterRequest,
  ClusterResponse,
  ClustersClientImpl,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

import { Rpc } from './types';

const MAX_BATCH_SIZE = 1000;

export interface BatchedClustersClientImplOpts {
  readonly service?: string;
  /**
   * Maximum number of requests in a given batch request. Defaults to 1000.
   */
  readonly maxBatchSize?: number;
}

/**
 * The same as `ClustersClientImpl` except that eligible RPC calls are batched
 * automatically. Only RPC calls made via the same client instance can be
 * batched together.
 */
export class BatchedClustersClientImpl extends ClustersClientImpl {
  private readonly autoBatchedCluster: (
    opt: BatchOption,
    req: ClusterRequest,
  ) => Promise<ClusterResponse>;

  constructor(rpc: Rpc, opts?: BatchedClustersClientImplOpts) {
    super(rpc, opts);
    const maxBatchSize = opts?.maxBatchSize || MAX_BATCH_SIZE;

    this.autoBatchedCluster = batched({
      fn: (req: ClusterRequest) => super.Cluster(req),
      shardFn: (req: ClusterRequest) => req.project,
      combineParamSets([req1], [req2]) {
        if (req1.testResults.length + req2.testResults.length > maxBatchSize) {
          return {
            ok: false,
            value: null,
          };
        }

        return {
          ok: true,
          value: [
            {
              project: req1.project,
              testResults: [...req1.testResults, ...req2.testResults],
            },
          ],
        };
      },
      splitReturn(paramSets, ret) {
        let pivot = 0;
        const splitRets: ClusterResponse[] = [];
        for (const [req] of paramSets) {
          const clusteredTestResults = ret.clusteredTestResults.slice(
            pivot,
            pivot + req.testResults.length,
          );

          splitRets.push({
            clusteredTestResults: clusteredTestResults,
            clusteringVersion: ret.clusteringVersion,
          });

          pivot += req.testResults.length;
        }

        return splitRets;
      },
    });
  }

  Cluster(
    request: ClusterRequest,
    opt: BatchOption = {},
  ): Promise<ClusterResponse> {
    return this.autoBatchedCluster(opt, request);
  }
}
