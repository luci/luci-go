// Copyright 2020 The LUCI Authors.
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

import { cached, CacheOption } from '@/common/libs/cached_fn';
import { PrpcClientExt } from '@/common/libs/prpc_client_ext';
import {
  Build,
  BuilderID,
  BuilderItem,
  GitilesCommit,
} from '@/common/services/buildbucket';

/**
 * Manually coded type definition and classes for milo internal service.
 * TODO(weiweilin): To be replaced by code generated version once we have one.
 * source: https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/milo/api/service/v1/rpc.proto
 */

export interface QueryBlamelistRequest {
  readonly gitilesCommit: GitilesCommit;
  readonly builder: BuilderID;
  readonly multiProjectSupport: boolean;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface CommitUser {
  readonly name: string;
  readonly email: string;
  readonly time: string;
}

export const enum CommitTreeDiffChangeType {
  Add = 'ADD',
  Copy = 'COPY',
  Delete = 'DELETE',
  Modify = 'MODIFY',
  Rename = 'RENAME',
}

export interface CommitTreeDiff {
  readonly type: CommitTreeDiffChangeType;
  readonly oldId: string;
  readonly oldMode: number;
  readonly oldPath: string;
  readonly newId: string;
  readonly newMode: number;
  readonly newPath: string;
}

export interface GitCommit {
  readonly id: string;
  readonly tree: string;
  readonly parents?: string[];
  readonly author: CommitUser;
  readonly committer: CommitUser;
  readonly message: string;
  readonly treeDiff: CommitTreeDiff[];
}

export interface QueryBlamelistResponse {
  readonly commits: GitCommit[];
  readonly nextPageToken?: string;
  readonly precedingCommit?: GitCommit;
}

export interface GetProjectCfgRequest {
  readonly project: string;
}

export interface Project {
  readonly logoUrl?: string;
  readonly bugUrlTemplate?: string;
  readonly metadataConfig?: MetadataConfig;
}

export interface MetadataConfig {
  readonly testMetadataProperties?: readonly DisplayRule[];
}
export interface DisplayRule {
  readonly schema: string;
  readonly displayItems: readonly DisplayItem[];
}

export interface DisplayItem {
  readonly displayName: string;
  readonly path: string;
}

export interface BugTemplate {
  readonly summary?: string;
  readonly description?: string;
  readonly monorailProject?: string;
  readonly components?: readonly string[];
}

export interface QueryRecentBuildsRequest {
  readonly builder: BuilderID;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface QueryRecentBuildsResponse {
  readonly builds?: readonly Build[];
  readonly nextPageToken?: string;
}

export interface ListBuildersRequest {
  readonly project?: string;
  readonly group?: string;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface ListBuildersResponse {
  readonly builders?: readonly BuilderItem[];
  readonly nextPageToken?: string;
}

export interface QueryBuilderStatsRequest {
  readonly builder: BuilderID;
}

export interface BuilderStats {
  readonly builder: BuilderID;
  readonly pendingBuildsCount?: number;
  readonly runningBuildsCount?: number;
}

export interface BatchCheckPermissionsRequest {
  readonly realm: string;
  readonly permissions: readonly string[];
}

export interface BatchCheckPermissionsResponse {
  readonly results: { readonly [key: string]: boolean };
}

export class MiloInternal {
  static readonly SERVICE = 'luci.milo.v1.MiloInternal';
  private readonly cachedCallFn: (
    opt: CacheOption,
    method: string,
    message: object
  ) => Promise<unknown>;

  constructor(client: PrpcClientExt) {
    this.cachedCallFn = cached(
      (method: string, message: object) =>
        client.call(MiloInternal.SERVICE, method, message),
      {
        key: (method, message) => `${method}-${stableStringify(message)}`,
      }
    );
  }

  async queryBlamelist(req: QueryBlamelistRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(
      cacheOpt,
      'QueryBlamelist',
      req
    )) as QueryBlamelistResponse;
  }

  async getProjectCfg(req: GetProjectCfgRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(cacheOpt, 'GetProjectCfg', req)) as Project;
  }

  async queryRecentBuilds(
    req: QueryRecentBuildsRequest,
    cacheOpt: CacheOption = {}
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'QueryRecentBuilds',
      req
    )) as QueryRecentBuildsResponse;
  }

  async listBuilders(req: ListBuildersRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(
      cacheOpt,
      'ListBuilders',
      req
    )) as ListBuildersResponse;
  }

  async queryBuilderStats(
    req: QueryBuilderStatsRequest,
    cacheOpt: CacheOption = {}
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'QueryBuilderStats',
      req
    )) as BuilderStats;
  }

  async batchCheckPermissions(
    req: BatchCheckPermissionsRequest,
    cacheOpt: CacheOption = {}
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'BatchCheckPermissions',
      req
    )) as BatchCheckPermissionsResponse;
  }
}
