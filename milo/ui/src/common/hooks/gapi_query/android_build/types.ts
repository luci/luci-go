// Copyright 2025 The LUCI Authors.
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

// Note that many of the types in this file can be auto-generated, but are not.
// This is because this is a private API and we only define what we need for our code.

export interface GetInvocationParams {
  /**
   * The id of the invocation to get.
   * E.g. "1234"
   */
  invocationId: string;
}

export interface InvocationResponse {
  /**
   * Unique ID. Assigned at creation time by the API backend.
   */
  resource: InvocationJson;
}

export interface InvocationJson {
  invocationId: string;
}

export enum SortingType {
  SORTING_TYPE_DEFAULT = 'SORTING_TYPE_DEFAULT',
  BUILD_ID = 'BUILD_ID',
  BASE_BUILD_ID = 'BASE_BUILD_ID',
  CREATION_TIMESTAMP = 'CREATION_TIMESTAMP',
  BUILD_ID_ASC = 'BUILD_ID_ASC',
}

export enum BuildLargeField {
  EXTRA_FIELD_UNSPECIFIED = 'EXTRA_FIELD_UNSPECIFIED',
  PINNED_MANIFEST = 'PINNED_MANIFEST',
}

export enum BuildStatus {
  PENDING = 'PENDING',
  SYNCING = 'SYNCING',
  SYNCED = 'SYNCED',
  BUILDING = 'BUILDING',
  BUILT = 'BUILT',
  TESTING = 'TESTING',
  COMPLETE = 'COMPLETE',
  ERROR = 'ERROR',
  PENDING_GERRIT_UPLOAD = 'PENDING_GERRIT_UPLOAD',
  ABANDONED = 'ABANDONED',
  POPPED = 'POPPED',
  NEW = 'NEW',
  COPIED = 'COPIED',
  PENDING_PRESYNC = 'PENDING_PRESYNC',
  PRESYNCING = 'PRESYNCING',
}

export enum SafeLevel {
  SAFE_LEVEL_UNSPECIFIED = 'SAFE_LEVEL_UNSPECIFIED',
  SAFE_0 = 'SAFE_0',
  SAFE_1 = 'SAFE_1',
  SAFE_2 = 'SAFE_2',
  PLATINUM = 'PLATINUM',
}

export interface Target {
  name: string;
}

export interface Change {
  host?: string;
  project?: string;
  branch?: string;
  changeNumber?: string; // int64
  patchset?: number; // int32
  status?: string; // Enum
  creationTime?: string; // int64
  lastModificationTime?: string; // int64
  latestRevision?: string;
  owner?: User;
  changeId?: string;
  topic?: string;
  submittedTime?: string; // int64
  originalSource?: string;
  isRapidChange?: boolean;
  projectPath?: string;
  changeType?: string; // Enum
  isBehindBlankMerge?: boolean;
}

export interface User {
  name?: string;
  email?: string;
  username?: string;
  accountId?: string; // int64
}

export interface Build {
  buildId: string;
  target: Target;
  change?: Change[];
  revision?: string;
  successful?: boolean;
  branch?: string;
  rank?: number; // int32
  signed?: boolean;
  releaseCandidateName?: string;
  creationTimestamp?: string; // int64
  buildAttemptStatus?: BuildStatus;
  previousBuildId?: string;
  referenceReleaseCandidateName?: string;
  worknodeId?: string;
  hasTests?: boolean;
  referenceBuildId?: string[];
  machineName?: string;
  completionTimestamp?: string; // int64
  baseBuild?: string;
  externalDiskName?: string;
  proofBuild?: boolean;
  fallbackInternal?: boolean;
  buildbotAvailableSpaceGb?: string; // int64
  archived?: boolean;
  externalId?: string;
  resetImageBuild?: boolean;
  buildbotSwVersion?: string;
  lastUpdatedTimestamp?: string; // int64
  safeLevel?: SafeLevel;
  worknodeAttemptId?: string;
  infraError?: boolean;
  changesFetchingDone?: boolean;
  fallbackErrorMsg?: string;
  triggerType?: string; // Enum
  promoted?: boolean;
  gitServerSeconds?: string; // int64
  gitLsremotes?: string; // int64
  vmImage?: string;
}

export interface ListBuildsRequest {
  branches?: string[];
  targets?: string[];
  start_creation_timestamp?: string; // Timestamp
  end_creation_timestamp?: string; // Timestamp
  sorting_type?: SortingType;
  page_token?: string;
  page_size?: number; // int32
  fields?: string;
}

export interface ListBuildsResponse {
  builds?: Build[];
  nextPageToken?: string;
  previousPageToken?: string;
}
