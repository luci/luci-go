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

import { PrpcClient } from '@chopsui/prpc-client';

/**
 * Manually coded type definition and classes for resultdb service.
 * TODO(weiweilin): To be replaced by code generated version once we have one.
 * source: https://chromium.googlesource.com/infra/luci/luci-go/+/ea9b54c38d87a4813576454fac9ac868bab8d9bc/buildbucket/proto/builder_service.proto
 */

export const enum Trinary {
  Unset = 0,
  Yes = 1,
  No = 2,
}

export interface GetBuildRequest {
  readonly id: string;
  readonly builder?: BuilderID;
  readonly buildNumber?: number;
  readonly fields?: string;
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
  readonly createdBy: string;
  readonly canceledBy?: string;
  readonly createTime: Timestamp;
  readonly startTime?: Timestamp;
  readonly endTime?: Timestamp;
  readonly updateTime: Timestamp;
  readonly status: BuildStatus;
  readonly summaryMarkdown?: string;
  readonly input: BuildInput;
  readonly output: BuildOutput;
  readonly steps?: Step[];
  readonly infra?: BuildInfra;
  readonly tags: StringPair[];
  readonly exe: Executable;
}

// This is from https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/buildbucket/proto/common.proto#25
export enum BuildStatus {
  Scheduled = 1,
  Started = 2,
  Success = 12,
  Failure = 20,
  InfraFailure = 36,
  Canceled = 68,
}

export interface BuildInput {
  readonly properties: {[key: string]: unknown};
  readonly gitilesCommit?: GitilesCommit;
  readonly gerritChanges?: GerritChange[];
  readonly experiments?: string[];
}

export interface GitilesCommit {
  readonly host: string;
  readonly project: string;
  readonly id: string;
  readonly ref: string;
  readonly position: number;
}

export interface GerritChange {
  readonly host: string;
  readonly project: string;
  readonly change: string;
  readonly patchset: string;
}

export interface BuildOutput {
  readonly properties: {[key: string]: unknown};
  readonly gitilesCommit: GitilesCommit;
  readonly logs: Log[];
}

export interface Log {
  readonly name: string;
  readonly viewUrl: string;
  readonly url: string;
}

export interface Step {
  readonly name: string;
  readonly startTime: Timestamp;
  readonly endTime?: Timestamp;
  readonly status: BuildStatus;
  readonly logs?: Log[];
  readonly summaryMarkdown?: string;
}

export interface BuildInfra {
  readonly buildbucket: BuildInfraBuildbucket;
  readonly swarming: BuildInfraSwarming;
  readonly logdog: BuildInfraLogDog;
  readonly recipe: BuildInfraRecipe;
  readonly resultdb?: BuildInfraResultdb;
}

export interface BuildInfraBuildbucket {
  readonly serviceConfigRevision: string;
  readonly requestedProperties: {[key: string]: unknown};
  readonly requestedDimensions: RequestedDimension[];
  readonly hostname: string;
}

export interface RequestedDimension {
  readonly key: string;
  readonly value: string;
  readonly expiration: string;
}

export interface BuildInfraSwarming {
  readonly hostname: string;
  readonly taskId?: string;
  readonly parentRunId: string;
  readonly taskServiceAccount: string;
  readonly priority: number;
  readonly taskDimensions: readonly RequestedDimension[];
  readonly botDimensions?: StringPair[];
  readonly caches: readonly BuildInfraSwarmingCacheEntry[];
  readonly containment: Containment;
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

export interface Containment {
  //TODO(weiweilin): implement
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
  readonly permitted: ResourcePermissions;
  readonly validityDuration: string;
}

export interface ResourcePermissions {
  readonly actions: readonly string[];
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
  experiments?: {[key: string]: boolean};
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

const BUILDS_SERVICE = 'buildbucket.v2.Builds';

export class BuildsService {
  private prpcClient: PrpcClient;

  constructor(readonly host: string, accessToken: string) {
    this.prpcClient = new PrpcClient({host, accessToken});
  }

  async cancelBuild(req: CancelBuildRequest) {
    return await this.call(
      'CancelBuild',
      req,
    ) as Build;
  }

  async scheduleBuild(req: ScheduleBuildRequest) {
    return await this.call(
      'ScheduleBuild',
      req,
    ) as Build;
  }

  private call(method: string, message: object) {
    return this.prpcClient.call(
      BUILDS_SERVICE,
      method,
      message,
    );
  }
}
