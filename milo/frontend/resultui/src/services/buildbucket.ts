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

import { cached, CacheOption } from '../libs/cached_fn';
import { PrpcClientExt } from '../libs/prpc_client_ext';
import { parseProtoDuration } from '../libs/time_utils';
import { timeout } from '../libs/utils';

/* eslint-disable max-len */
/**
 * Manually coded type definition and classes for buildbucket services.
 * TODO(weiweilin): To be replaced by code generated version once we have one.
 * source: https://chromium.googlesource.com/infra/luci/luci-go/+/04a118946d13ad326c44dba9a635116ff7f31c4e/buildbucket/proto/builds_service.proto
 */
/* eslint-enable max-len */

export const TEST_PRESENTATION_KEY = '$recipe_engine/resultdb/test_presentation';
export const BLAMELIST_PIN_KEY = '$recipe_engine/milo/blamelist_pins';

export const BUILD_FIELD_MASK =
  'id,builder,number,canceledBy,createTime,startTime,endTime,cancelTime,status,summaryMarkdown,input,output,steps,' +
  'infra.buildbucket.agent,infra.swarming,infra.resultdb,tags,exe,schedulingTimeout,executionTimeout,gracePeriod';

// Includes: id, builder, number, createTime, startTime, endTime, status, summaryMarkdown.
export const SEARCH_BUILD_FIELD_MASK =
  'builds.*.id,builds.*.builder,builds.*.number,builds.*.createTime,builds.*.startTime,builds.*.endTime,' +
  'builds.*.status,builds.*.summaryMarkdown';

export const enum Trinary {
  Unset = 'UNSET',
  Yes = 'YES',
  No = 'NO',
}

export interface GetBuildRequest {
  readonly id?: string;
  readonly builder?: BuilderID;
  readonly buildNumber?: number;
  readonly fields?: string;
}

export interface SearchBuildsRequest {
  readonly predicate: BuildPredicate;
  readonly pageSize?: number;
  readonly pageToken?: string;
  readonly fields?: string;
}

export interface SearchBuildsResponse {
  readonly builds: readonly Build[];
  readonly nextPageToken?: string;
}

export interface BuildPredicate {
  readonly builderId?: BuilderID;
  readonly status?: BuildStatus;
  readonly gerritChanges?: readonly GerritChange[];
  readonly createdBy?: string;
  readonly tags?: readonly StringPair[];
  readonly build?: BuildRange;
  readonly experiments?: readonly string[];
}

export interface BuildRange {
  readonly startBuildId: string;
  readonly endBuildId: string;
}

export interface BuilderID {
  readonly project: string;
  readonly bucket: string;
  readonly builder: string;
}

export interface Timestamp {
  readonly seconds: number;
  readonly nanos: number;
}

export interface Build {
  readonly id: string;
  readonly builder: BuilderID;
  readonly number?: number;
  readonly canceledBy?: string;
  readonly createTime: string;
  readonly startTime?: string;
  readonly endTime?: string;
  readonly cancelTime?: string;
  readonly status: BuildStatus;
  readonly summaryMarkdown?: string;
  readonly input?: BuildInput;
  readonly output?: BuildOutput;
  readonly steps?: readonly Step[];
  readonly infra?: BuildInfra;
  readonly tags: readonly StringPair[];
  readonly exe: Executable;
  readonly schedulingTimeout?: string;
  readonly executionTimeout?: string;
  readonly gracePeriod?: string;
}

// This is from https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/buildbucket/proto/common.proto#25
export enum BuildStatus {
  Scheduled = 'SCHEDULED',
  Started = 'STARTED',
  Success = 'SUCCESS',
  Failure = 'FAILURE',
  InfraFailure = 'INFRA_FAILURE',
  Canceled = 'CANCELED',
}

export interface TestPresentationConfig {
  /**
   * A list of keys that will be rendered as columns in the test results tab.
   * status is always the first column and name is always the last column (you
   * don't need to specify them).
   *
   * A key must be one of the following:
   * 1. 'v.{variant_key}': variant.def[variant_key] of the test variant (e.g.
   * v.gpu).
   */
  column_keys?: string[];
  /**
   * A list of keys that will be used for grouping test variants in the test
   * results tab.
   *
   * A key must be one of the following:
   * 1. 'status': status of the test variant.
   * 2. 'name': test_metadata.name of the test variant.
   * 3. 'v.{variant_key}': variant.def[variant_key] of the test variant (e.g.
   * v.gpu).
   *
   * Caveat: test variants with only expected results are not affected by this
   * setting and are always in their own group.
   */
  grouping_keys?: string[];
}

export interface BuildInput {
  readonly properties?: {
    [TEST_PRESENTATION_KEY]?: TestPresentationConfig;
    [key: string]: unknown;
  };
  readonly gitilesCommit?: GitilesCommit;
  readonly gerritChanges?: GerritChange[];
  readonly experiments?: string[];
}

export interface GitilesCommit {
  readonly host: string;
  readonly project: string;
  readonly id?: string;
  readonly ref?: string;
  readonly position: number;
}

export interface GerritChange {
  readonly host: string;
  readonly project: string;
  readonly change: string;
  readonly patchset: string;
}

export interface BuildOutput {
  readonly properties?: {
    [TEST_PRESENTATION_KEY]?: TestPresentationConfig;
    [BLAMELIST_PIN_KEY]?: GitilesCommit[];
    [key: string]: unknown;
  };
  readonly gitilesCommit?: GitilesCommit;
  readonly logs: Log[];
}

export interface Log {
  readonly name: string;
  readonly viewUrl: string;
  readonly url: string;
}

export interface Step {
  readonly name: string;
  readonly startTime?: string;
  readonly endTime?: string;
  readonly status: BuildStatus;
  readonly logs?: Log[];
  readonly summaryMarkdown?: string;
}

export interface BuildInfra {
  readonly swarming: BuildInfraSwarming;
  readonly resultdb?: BuildInfraResultdb;
  readonly buildbucket?: BuildInfraBuildbucket;
}

export interface BuildInfraBuildbucket {
  readonly serviceConfigRevision: string;
  readonly requestedProperties: { [key: string]: unknown };
  readonly requestedDimensions: RequestedDimension[];
  readonly hostname: string;
  readonly agent?: BuildAgent;
}

export interface BuildAgent {
  readonly input: BuildAgentInput;
  readonly output?: BuildAgentOutput;
}

export interface BuildAgentInput {
  readonly data: {[key:string]: BuildAgentInputDataRef};
}


export interface BuildAgentInputDataRef {
  readonly cipd: Cipd;
  readonly onPath: string[];
}

export interface BuildAgentOutput {
  readonly resolvedData: {[key:string]: BuildAgentResolvedDataRef};
  readonly status: BuildStatus;
  readonly summaryHtml: string;
  readonly agentPlatform: string;
  readonly totalDuration: string;
}

export interface BuildAgentResolvedDataRef {
  readonly cipd: Cipd;
}

export interface Cipd {
  readonly specs: PkgSpec[];
}

export interface PkgSpec {
  readonly package: string;
  readonly version: string;
}

export interface RequestedDimension {
  readonly key: string;
  readonly value: string;
  readonly expiration: string;
}

export interface BuildInfraSwarming {
  readonly hostname: string;
  readonly taskId?: string;
  readonly parentRunId?: string;
  readonly taskServiceAccount: string;
  readonly priority: number;
  readonly taskDimensions: readonly RequestedDimension[];
  readonly botDimensions?: StringPair[];
  readonly caches: readonly BuildInfraSwarmingCacheEntry[];
}

export interface StringPair {
  readonly key: string;
  readonly value: string;
}

export interface BuildInfraSwarmingCacheEntry {
  readonly name: string;
  readonly path: string;
  readonly waitForWarmCache: string;
  readonly envVar: string;
}

export interface BuildInfraLogDog {
  readonly hostname: string;
  readonly project: string;
  readonly prefix: string;
}

export interface BuildInfraRecipe {
  readonly cipdPackage: string;
  readonly name: string;
}

export interface BuildInfraResultdb {
  readonly hostname: string;
  readonly invocation?: string;
}

export interface Executable {
  readonly cipdPackage: string;
  readonly cipdVersion: string;
  readonly cmd: readonly string[];
}

export interface PermittedActionsRequest {
  readonly resourceKind: string;
  readonly resourceIds: readonly string[];
}

export interface PermittedActionsResponse {
  permitted: { [key: string]: ResourcePermissions };
  validityDuration: string;
}

export interface ResourcePermissions {
  readonly actions?: readonly string[];
}

export interface CancelBuildRequest {
  id: string;
  summaryMarkdown: string;
  fields?: string;
}

export interface ScheduleBuildRequest {
  requestId?: string;
  templateBuildId?: string;
  builder?: BuilderID;
  experiments?: { [key: string]: boolean };
  properties?: {};
  gitilesCommit?: GitilesCommit;
  gerritChanges?: GerritChange[];
  tags?: StringPair[];
  dimensions?: RequestedDimension[];
  priority?: string;
  notify?: Notification;
  fields?: string;
  critical?: Trinary;
  exe?: Executable;
  swarming?: {
    parentRunId: string;
  };
}

export class BuildsService {
  private static SERVICE = 'buildbucket.v2.Builds';
  private readonly cachedCallFn: (opt: CacheOption, method: string, message: object) => Promise<unknown>;

  constructor(client: PrpcClientExt) {
    this.cachedCallFn = cached(
      (method: string, message: object) => client.call(BuildsService.SERVICE, method, message),
      {
        key: (method, message) => `${method}-${stableStringify(message)}`,
      }
    );
  }

  async getBuild(req: GetBuildRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(cacheOpt, 'GetBuild', req)) as Build;
  }

  async searchBuilds(req: SearchBuildsRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(cacheOpt, 'SearchBuilds', req)) as SearchBuildsResponse;
  }

  async cancelBuild(req: CancelBuildRequest) {
    return (await this.cachedCallFn({ acceptCache: false, skipUpdate: true }, 'CancelBuild', req)) as Build;
  }

  async scheduleBuild(req: ScheduleBuildRequest) {
    return (await this.cachedCallFn({ acceptCache: false, skipUpdate: true }, 'ScheduleBuild', req)) as Build;
  }
}

export interface GetBuilderRequest {
  readonly id: BuilderID;
}

export interface Builder {
  descriptionHtml?: string;
}

export interface BuilderItem {
  readonly id: BuilderID;
  readonly config: Builder;
}

export class BuildersService {
  private static SERVICE = 'buildbucket.v2.Builders';

  private readonly cachedCallFn: (opt: CacheOption, method: string, message: object) => Promise<unknown>;

  constructor(client: PrpcClientExt) {
    this.cachedCallFn = cached(
      (method: string, message: object) => client.call(BuildersService.SERVICE, method, message),
      { key: (method, message) => `${method}-${stableStringify(message)}` }
    );
  }

  async getBuilder(req: GetBuilderRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(cacheOpt, 'GetBuilder', req)) as BuilderItem;
  }
}

export class AccessService {
  private static SERVICE = 'access.Access';

  private readonly cachedPermittedActionsFn: (
    cacheOpt: CacheOption,
    req: PermittedActionsRequest
  ) => Promise<PermittedActionsResponse>;

  constructor(client: PrpcClientExt) {
    // TODO(weiweilin): access service can be unreliable.
    // Try to cache this in service worker or localStorage.
    this.cachedPermittedActionsFn = cached(
      async (req: PermittedActionsRequest) => {
        return (await client.call(AccessService.SERVICE, 'PermittedActions', req)) as PermittedActionsResponse;
      },
      {
        key: (req) => stableStringify(req),
        expire: async (_req, resPromise) => {
          const res = await resPromise;
          await timeout(parseProtoDuration(res.validityDuration));
          return;
        },
      }
    );
  }

  async permittedActions(req: PermittedActionsRequest, cacheOpt: CacheOption = {}) {
    return this.cachedPermittedActionsFn(cacheOpt, req);
  }
}
