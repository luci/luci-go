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
  change_number?: string; // int64
  patchset?: number; // int32
  status?: string; // Enum
  creation_time?: string; // int64
  last_modification_time?: string; // int64
  latest_revision?: string;
  owner?: User;
  change_id?: string;
  topic?: string;
  submitted_time?: string; // int64
  original_source?: string;
  is_rapid_change?: boolean;
  project_path?: string;
  change_type?: string; // Enum
  is_behind_blank_merge?: boolean;
}

export interface User {
  name?: string;
  email?: string;
  username?: string;
  account_id?: string; // int64
}

export interface Build {
  build_id: string;
  target: Target;
  change?: Change[];
  revision?: string;
  successful?: boolean;
  branch?: string;
  rank?: number; // int32
  signed?: boolean;
  release_candidate_name?: string;
  creation_timestamp?: string; // int64
  build_attempt_status?: BuildStatus;
  previous_build_id?: string;
  reference_release_candidate_name?: string;
  worknode_id?: string;
  has_tests?: boolean;
  reference_build_id?: string[];
  machine_name?: string;
  completion_timestamp?: string; // int64
  base_build?: string;
  external_disk_name?: string;
  proof_build?: boolean;
  fallback_internal?: boolean;
  buildbot_available_space_gb?: string; // int64
  archived?: boolean;
  external_id?: string;
  reset_image_build?: boolean;
  buildbot_sw_version?: string;
  last_updated_timestamp?: string; // int64
  safe_level?: SafeLevel;
  worknode_attempt_id?: string;
  infra_error?: boolean;
  changes_fetching_done?: boolean;
  fallback_error_msg?: string;
  trigger_type?: string; // Enum
  promoted?: boolean;
  git_server_seconds?: string; // int64
  git_lsremotes?: string; // int64
  vm_image?: string;
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
  next_page_token?: string;
  previous_page_token?: string;
}
