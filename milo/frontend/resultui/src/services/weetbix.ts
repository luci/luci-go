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

import stableStringify from 'fast-json-stable-stringify';

import { batched, BatchOption } from '../libs/batched_fn';
import { cached, CacheOption } from '../libs/cached_fn';
import { PrpcClientExt } from '../libs/prpc_client_ext';

export interface FailureReason {
  readonly primaryErrorMessage: string;
}

export interface ClusterRequest {
  readonly project: string;
  readonly testResults: ReadonlyArray<{
    readonly requestTag?: string;
    readonly testId: string;
    readonly failureReason?: FailureReason;
  }>;
}

export interface Cluster {
  readonly clusterId: ClusterId;
  readonly bug?: AssociatedBug;
}

export interface ClusterResponse {
  readonly clusteredTestResults: ReadonlyArray<{
    readonly requestTag?: string;
    readonly clusters: readonly Cluster[];
  }>;
  readonly clusteringVersion: ClusteringVersion;
}

export interface ClusteringVersion {
  readonly algorithmsVersion: string;
  readonly rulesVersion: string;
  readonly configVersion: string;
}

export interface ClusterId {
  readonly algorithm: string;
  readonly id: string;
}

export interface AssociatedBug {
  readonly system: string;
  readonly id: string;
  readonly linkText: string;
  readonly url: string;
}

export class ClustersService {
  private static SERVICE = 'weetbix.v1.Clusters';

  private readonly cachedBatchedCluster: (
    cacheOpt: CacheOption,
    batchOpt: BatchOption,
    req: ClusterRequest
  ) => Promise<ClusterResponse>;

  constructor(client: PrpcClientExt) {
    const CLUSTER_BATCH_LIMIT = 1000;

    const batchedCluster = batched<[ClusterRequest], ClusterResponse>({
      fn: (req: ClusterRequest) => client.call(ClustersService.SERVICE, 'Cluster', req),
      combineParamSets: ([req1], [req2]) => {
        const canCombine =
          req1.testResults.length + req2.testResults.length <= CLUSTER_BATCH_LIMIT && req1.project === req2.project;
        if (!canCombine) {
          return { ok: false } as ResultErr<void>;
        }
        return {
          ok: true,
          value: [
            {
              project: req1.project,
              testResults: [...req1.testResults, ...req2.testResults],
            },
          ] as [ClusterRequest],
        };
      },
      splitReturn: (paramSets, ret) => {
        let pivot = 0;
        const splitRets: ClusterResponse[] = [];
        for (const [req] of paramSets) {
          splitRets.push({
            clusteringVersion: ret.clusteringVersion,
            clusteredTestResults: ret.clusteredTestResults.slice(pivot, pivot + req.testResults.length),
          });
          pivot += req.testResults.length;
        }

        return splitRets;
      },
    });

    this.cachedBatchedCluster = cached((batchOpt: BatchOption, req: ClusterRequest) => batchedCluster(batchOpt, req), {
      key: (_batchOpt, req) => stableStringify(req),
    });
  }

  async cluster(req: ClusterRequest, batchOpt: BatchOption = {}, cacheOpt: CacheOption = {}): Promise<ClusterResponse> {
    return (await this.cachedBatchedCluster(cacheOpt, batchOpt, req)) as ClusterResponse;
  }
}

/**
 * Construct a link to a weetbix rule page.
 */
export function makeRuleLink(project: string, ruleId: string) {
  return `https://${CONFIGS.WEETBIX.HOST}/p/${project}/rules/${ruleId}`;
}

/**
 * Construct a link to a weetbix cluster.
 */
export function makeClusterLink(project: string, clusterId: ClusterId) {
  return `https://${CONFIGS.WEETBIX.HOST}/p/${project}/clusters/${clusterId.algorithm}/${clusterId.id}`;
}
