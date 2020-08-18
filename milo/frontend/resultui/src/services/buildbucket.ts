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

export interface GetBuildRequest {
  readonly id: string;
  readonly builder?: BuilderID;
  readonly build_number?: number;
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
  readonly number: string;
  readonly created_by: string;
  readonly canceled_by: string;
  readonly create_time: Timestamp;
  readonly start_time: Timestamp;
  readonly end_time: Timestamp;
  readonly update_time: Timestamp;
  readonly status: BuildStatus;
  readonly summary_markdown?: string;
  readonly input: BuildInput;
  readonly output: BuildOutput;
  readonly steps: Step[];
  readonly infra?: BuildInfra;
  readonly tags: StringPair[];
  readonly exe: Executable;
}

// This is from https://source.chromium.org/chromium/infra/infra/+/master:go/src/go.chromium.org/luci/buildbucket/proto/common.proto;l=27
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
  readonly gitiles_commit?: GitilesCommit;
  readonly gerrit_changes?: GerritChange[];
  readonly experimental: boolean;
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
  readonly gitiles_commit: GitilesCommit;
  readonly logs: Log[];
}

export interface Log {
  readonly name: string;
  readonly view_url: string;
  readonly url: string;
}

export interface Step {
  readonly name: string;
  readonly start_time: string;
  readonly end_time: string;
  readonly status: BuildStatus;
  readonly logs: Log[];
  readonly summary_markdown: string;
}

export interface BuildInfra {
  readonly buildbucket: BuildInfraBuildbucket;
  readonly swarming: BuildInfraSwarming;
  readonly log_dog: BuildInfraLogDog;
  readonly recipe: BuildInfraRecipe;
  readonly resultdb: BuildInfraResultdb;
}

export interface BuildInfraBuildbucket {
  readonly service_config_revision: string;
  readonly requested_properties: {[key: string]: unknown};
  readonly requested_dimensions: RequestedDimension[];
  readonly hostname: string;
}

export interface RequestedDimension {
  readonly key: string;
  readonly value: string;
  readonly expiration: string;
}

export interface BuildInfraSwarming {
  readonly hostname: string;
  readonly task_id?: string;
  readonly parent_run_id: string;
  readonly task_service_account: string;
  readonly priority: number;
  readonly task_dimensions: readonly RequestedDimension[];
  readonly bot_dimensions?: StringPair[];
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
  readonly wait_for_warm_cache: string;
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
  readonly cipd_package: string;
  readonly name: string;
}

export interface BuildInfraResultdb {
  readonly hostname: string;
  readonly invocation: string;
}

export interface Executable {
  readonly cipd_package: string;
  readonly cipd_version: string;
  readonly cmd: readonly string[];
}

export interface PermittedActionsRequest {
  readonly resource_kind: string;
  readonly resource_ids: readonly string[];
}

export interface PermittedActionsResponse {
  readonly permitted: ResourcePermissions;
  readonly validity_duration: string;
}

export interface ResourcePermissions {
  readonly actions: readonly string[];
}
