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
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import {
  BatchRequest,
  BatchResponse,
  BuildsClientImpl,
  CancelBuildRequest,
  GetBuildRequest,
  GetBuildStatusRequest,
  ScheduleBuildRequest,
  SearchBuildsRequest,
  SearchBuildsResponse,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

import { Rpc } from './types';

const MAX_BATCH_SIZE = 200;

export interface BatchedBuildsClientImplOpts {
  readonly service?: string;
  /**
   * Maximum number of requests in a given batch request. Defaults to 200.
   */
  readonly maxBatchSize?: number;
}

/**
 * The same as `BuildsClientImpl` except that eligible RPC calls are batched
 * automatically. Only RPC calls made via the same client instance can be
 * batched together.
 */
export class BatchedBuildsClientImpl extends BuildsClientImpl {
  private readonly autoBatchedBatch: (
    opt: BatchOption,
    req: BatchRequest,
  ) => Promise<BatchResponse>;

  constructor(rpc: Rpc, opts?: BatchedBuildsClientImplOpts) {
    super(rpc, opts);
    const maxBatchSize = opts?.maxBatchSize || MAX_BATCH_SIZE;

    this.autoBatchedBatch = batched({
      // eslint-disable-next-line new-cap
      fn: (req: BatchRequest) => super.Batch(req),
      combineParamSets([req1], [req2]) {
        if (req1.requests.length + req2.requests.length > maxBatchSize) {
          return {
            ok: false,
            value: null,
          };
        }

        return {
          ok: true,
          value: [
            {
              requests: [...req1.requests, ...req2.requests],
            },
          ],
        };
      },
      splitReturn(paramSets, ret) {
        let pivot = 0;
        const splitRets: BatchResponse[] = [];
        for (const [req] of paramSets) {
          splitRets.push({
            responses: ret.responses.slice(pivot, pivot + req.requests.length),
          });
          pivot += req.requests.length;
        }

        return splitRets;
      },
    });
  }

  async GetBuild(
    request: GetBuildRequest,
    opt: BatchOption = {},
  ): Promise<Build> {
    const batchedRes = await this.autoBatchedBatch(opt, {
      requests: [
        {
          getBuild: request,
        },
      ],
    });
    // The responses array should always have the same length as the request
    // array.
    const res = batchedRes.responses[0];
    if (res.error) {
      throw res.error;
    }
    return res.getBuild!;
  }

  async SearchBuilds(
    request: SearchBuildsRequest,
    opt: BatchOption = {},
  ): Promise<SearchBuildsResponse> {
    const batchedRes = await this.autoBatchedBatch(opt, {
      requests: [
        {
          searchBuilds: request,
        },
      ],
    });
    // The responses array should always have the same length as the request
    // array.
    const res = batchedRes.responses[0];
    if (res.error) {
      throw res.error;
    }
    return res.searchBuilds!;
  }

  async ScheduleBuild(
    request: ScheduleBuildRequest,
    opt: BatchOption = {},
  ): Promise<Build> {
    const batchedRes = await this.autoBatchedBatch(opt, {
      requests: [
        {
          scheduleBuild: request,
        },
      ],
    });
    // The responses array should always have the same length as the request
    // array.
    const res = batchedRes.responses[0];
    if (res.error) {
      throw res.error;
    }
    return res.scheduleBuild!;
  }

  async CancelBuild(
    request: CancelBuildRequest,
    opt: BatchOption = {},
  ): Promise<Build> {
    const batchedRes = await this.autoBatchedBatch(opt, {
      requests: [
        {
          cancelBuild: request,
        },
      ],
    });
    // The responses array should always have the same length as the request
    // array.
    const res = batchedRes.responses[0];
    if (res.error) {
      throw res.error;
    }
    return res.cancelBuild!;
  }

  async GetBuildStatus(
    request: GetBuildStatusRequest,
    opt: BatchOption = {},
  ): Promise<Build> {
    const batchedRes = await this.autoBatchedBatch(opt, {
      requests: [
        {
          getBuildStatus: request,
        },
      ],
    });
    // The responses array should always have the same length as the request
    // array.
    const res = batchedRes.responses[0];
    if (res.error) {
      throw res.error;
    }
    return res.getBuildStatus!;
  }

  Batch(request: BatchRequest, opt: BatchOption = {}): Promise<BatchResponse> {
    return this.autoBatchedBatch(opt, request);
  }
}
