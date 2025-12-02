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

import { UseQueryResult } from '@tanstack/react-query';

import { WrapperQueryOptions } from '@/common/types/query_wrapper_options';

import { useGapiQuery } from '../gapi_query';

// Base path for the Realtime Stability service.
// This is a guess based on the pattern, may need adjustment.
const API_BASE_PATH_ROOT =
  'https://androidrealtimestability.pa.googleapis.com/';

// ===================================================================
// Shared Types
// ===================================================================

export enum FileType {
  UNKNOWN = 'UNKNOWN',
  LOGCAT_TEXT = 'LOGCAT_TEXT',
  ANR_TEXT = 'ANR_TEXT',
  BUGREPORT_ZIP_ANR = 'BUGREPORT_ZIP_ANR',
  BUGREPORT_ZIP_TOMBSTONE = 'BUGREPORT_ZIP_TOMBSTONE',
  TOMBSTONE_ZIP = 'TOMBSTONE_ZIP',
  BUGREPORT_TEXT = 'BUGREPORT_TEXT',
  MOBLY_ZIP_LOGCAT = 'MOBLY_ZIP_LOGCAT',
  OXYGEN_ZIP_LOGCAT = 'OXYGEN_ZIP_LOGCAT',
  KERNEL_LOG_TEXT = 'KERNEL_LOG_TEXT',
  BUGREPORT_ZIP_KERNEL_LOG = 'BUGREPORT_ZIP_KERNEL_LOG',
}

export enum CrashAnnotation {
  CRASH_ANNOTATION_UNKNOWN = 'CRASH_ANNOTATION_UNKNOWN',
  CRASH_ANNOTATION_CRITICAL_CRASH = 'CRASH_ANNOTATION_CRITICAL_CRASH',
}

export enum LogType {
  UNKNOWN_LOG_TYPE = 'UNKNOWN_LOG_TYPE',
  OTHER = 'OTHER',
  STACKTRACE = 'STACKTRACE',
  NATIVE_CRASH = 'NATIVE_CRASH',
  JAVA_CRASH = 'JAVA_CRASH',
  WATCHDOG_KILL_CRASH = 'WATCHDOG_KILL_CRASH',
  ANR = 'ANR',
  KERNEL_CRASH = 'KERNEL_CRASH',
  PREWATCHDOG = 'PREWATCHDOG',
  WATCHDOG = 'WATCHDOG',
}

export interface BuildInfo {
  build_id: string;
  build_target: string;
  build_branch: string;
}

export interface TestCaseInfo {
  method_name: string;
  module_name: string;
  started_time?: string; // google.protobuf.Timestamp
  finished_time?: string; // google.protobuf.Timestamp
  failed_time?: string; // google.protobuf.Timestamp
}

export interface LogcatRecord {
  record_id: string;
  raw_text: string;
  cleaned_text: string;
  log_type: LogType;
  timestamp: string; // int64
  count: string; // int64
  throwing_thread: string;
  tag: string;
  severity: string; // ads.common.testing.analysis.traps.LogEntry.LogLevel
  test_case_info?: TestCaseInfo;
  process_name: string;
  crash_annotations: CrashAnnotation[];
  line_number: string; // int64
}

export interface LogcatRecords {
  filename: string;
  log_records: LogcatRecord[];
  sub_filename: string;
  device_build_info?: BuildInfo;
  file_type: FileType;
  work_unit_id: string;
  test_result_id: string;
}

export interface TestDiscriminator {
  build_target: string;
  run_target: string;
  branch: string;
  cluster: string;
  atp_test_name: string;
}

export interface ErrorSnippet {
  ants_invocation_id: string;
  build_id: string;
  invocation_timestamp_usec: string; // int64
  test_discriminator?: TestDiscriminator;
  passed: boolean;
  log_records?: LogcatRecords;
}

// ===================================================================
// GetErrorSnippetsForAnTSInvocation
// ===================================================================

export interface GetErrorSnippetForAnTSInvocationRequest {
  invocation_id: string;
}

export interface GetErrorSnippetResponse {
  error_snippets: ErrorSnippet[];
}

/**
 * Gets error snippets in Android logcat files for a given AnTS invocation.
 */
export const useGetErrorSnippetsForAnTSInvocation = (
  params: GetErrorSnippetForAnTSInvocationRequest,
  queryOptions: WrapperQueryOptions<GetErrorSnippetResponse>,
): UseQueryResult<GetErrorSnippetResponse> => {
  const path = `v1/errorsnippet/${params.invocation_id}`;
  return useGapiQuery<GetErrorSnippetResponse>(
    {
      method: 'GET',
      path: `${API_BASE_PATH_ROOT}${path}`,
    },
    queryOptions,
  );
};
