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

// It is assumed that the base path for the Barrelman service is the default
// Google APIs root. A more specific host may be required.
const API_BASE_PATH_ROOT =
  'https://androidtestresultsfluxgate.pa.googleapis.com/';

// Note that many of the types in this file are manually defined from the proto
// as this is a private API and we only define what we need.

// ===================================================================
// Shared Types
// ===================================================================

export interface TestResultIdInfo {
  test_result_id: string;
  build_id: string;
  invocation_id: string;
}

export interface AggregatedCluster {
  cluster_id: string;
  count: string; // Proto is uint64
  exemplar_result?: TestResultIdInfo;
  first_seen_result?: TestResultIdInfo;
}

export enum PresubmitBlockingStatus {
  PRESUBMIT_UNSPECIFIED = 'PRESUBMIT_UNSPECIFIED',
  PRESUBMIT_BLOCKING = 'PRESUBMIT_BLOCKING',
  PRESUBMIT_WARNING = 'PRESUBMIT_WARNING',
  PRESUBMIT_INFORMATIONAL = 'PRESUBMIT_INFORMATIONAL',
  PRESUBMIT_DISABLED = 'PRESUBMIT_DISABLED',
}

export enum SLOStatus {
  UNSET = 'UNSET',
  OK = 'OK',
  OUT_OF_SLO = 'OUT_OF_SLO',
  UNSURE = 'UNSURE',
}

export interface FailRate {
  failures: string; // Proto is uint64
  total: string; // Proto is uint64
  rate: number;
  distribution?: {
    percentile: number;
    rate: number;
  }[];
}

export interface TestHealth {
  fail_rate?: FailRate;
  fail_rate_before_retries?: FailRate;
  fail_rate_after_retries?: FailRate;
  droid_gardener?: boolean;
  demoted?: boolean;
  blocking_status?: PresubmitBlockingStatus;
  flake_slo?: SLOStatus;
}

export interface TransitionInterval {
  percentiles?: {
    percentile: number;
    result?: TestResultIdInfo;
  }[];
}

export interface SegmentSummary {
  health?: TestHealth;
  clusters: AggregatedCluster[];
  start_result?: TestResultIdInfo;
  end_result?: TestResultIdInfo;
  start_exemplar_result?: TestResultIdInfo;
  end_exemplar_result?: TestResultIdInfo;
  transition_interval?: TransitionInterval;
}

export interface Changepoint {
  before_segment?: SegmentSummary;
  after_segment?: SegmentSummary;
  last_week_presubmit_metrics?: TestHealth;
}

export interface FluxgateSequence {
  start: string; // Proto is uint64
  end: string; // Proto is uint64
  start_exemplar?: string; // Proto is uint64
  end_exemplar?: string; // Proto is uint64
  passes: string; // Proto is uint64
  build_count: string; // Proto is uint64
  num_passes_on_retry?: string; // Proto is uint64
  incomplete_fails?: string; // Proto is uint64
  clusters: AggregatedCluster[];
  start_result?: TestResultIdInfo;
  end_result?: TestResultIdInfo;
  start_exemplar_result?: TestResultIdInfo;
  end_exemplar_result?: TestResultIdInfo;
}

export interface FluxgateSequences {
  sequences: FluxgateSequence[];
  build_before_changepoint: string; // Proto is uint64
  previous: FluxgateSequence[];
  changepoint_uncertainty_interval?: {
    percentile: number;
    result?: TestResultIdInfo;
  }[];
  significance: number;
  threshold: number;
  presubmit_blocking_status?: PresubmitBlockingStatus;
}

export interface TestResultFluxgateStatus {
  sequences?: FluxgateSequences;
  branch: string;
  target: string;
  // TestDefinition and TestIdentifier omitted as they are complex and not fully defined
  updated_timestamp_ms: string; // Proto is uint64
  // Status omitted
  test_identifier_id: string;
  changepoint?: Changepoint;
}

export interface SegmentSummaries {
  test_identifier_id: string;
  summaries: SegmentSummary[];
  combination_result?:
    | 'COMBINATION_RESULT_UNSPECIFIED'
    | 'NO_COMBINATION'
    | 'COMBINATION_APPLIED'
    | 'COMBINATION_MISSING_DATA';
}

// ===================================================================
// GetTestResultFluxgateStatus
// ===================================================================

export interface GetTestResultFluxgateStatusRequest {
  test_identifier_ids: string[];
  ref_build_id?: string;
  max_staleness?: string; // Proto is uint64
  include_flake_slo_calculation?: boolean;
}

export interface GetTestResultFluxgateStatusResponse {
  status: TestResultFluxgateStatus[];
}

/**
 * Gets the latest fluxgate sequence for a test identifier id + ref build id.
 */
export const useGetTestResultFluxgateStatus = (
  params: GetTestResultFluxgateStatusRequest,
  queryOptions: WrapperQueryOptions<GetTestResultFluxgateStatusResponse>,
): UseQueryResult<GetTestResultFluxgateStatusResponse> => {
  const path = 'v1/testResultFluxgateStatus';
  return useGapiQuery<GetTestResultFluxgateStatusResponse>(
    {
      method: 'GET',
      path: `${API_BASE_PATH_ROOT}${path}`,
      params: {
        test_identifier_ids: params.test_identifier_ids,
        ref_build_id: params.ref_build_id,
        max_staleness: params.max_staleness,
        include_flake_slo_calculation: params.include_flake_slo_calculation,
      },
    },
    queryOptions,
  );
};

// ===================================================================
// GetTestResultFluxgateSegmentSummaries
// ===================================================================

export interface ResultRange {
  starting_build_id?: string;
  ending_build_id?: string;
}

export interface CombinationStrategy {
  combine_overlapping?: boolean;
  fail_rate_similarity_threshold?: number;
  segment_size_threshold?: number; // Proto is uint32
}

export interface GetTestResultFluxgateSegmentSummariesRequest {
  test_identifier_ids: string[];
  range?: ResultRange;
  combination_strategy?: CombinationStrategy;
  max_staleness?: string; // Proto is uint64
}

export interface GetTestResultFluxgateSegmentSummariesResponse {
  summaries: SegmentSummaries[];
}

/**
 * Gets the fluxgate SegmentSummaries for a list of test identifier ids and range.
 */
export const useGetTestResultFluxgateSegmentSummaries = (
  params: GetTestResultFluxgateSegmentSummariesRequest,
  queryOptions: WrapperQueryOptions<GetTestResultFluxgateSegmentSummariesResponse>,
): UseQueryResult<GetTestResultFluxgateSegmentSummariesResponse> => {
  // The 'v1' prefix is already part of the GET path in the proto definition.
  const path = 'v1/testResultFluxgateSegmentSummaries';
  return useGapiQuery<GetTestResultFluxgateSegmentSummariesResponse>(
    {
      method: 'GET',
      path: `${API_BASE_PATH_ROOT}${path}`,
      params: {
        test_identifier_ids: params.test_identifier_ids,
        'range.starting_build_id': params.range?.starting_build_id,
        'range.ending_build_id': params.range?.ending_build_id,
        'combination_strategy.combine_overlapping':
          params.combination_strategy?.combine_overlapping,
        'combination_strategy.fail_rate_similarity_threshold':
          params.combination_strategy?.fail_rate_similarity_threshold,
        'combination_strategy.segment_size_threshold':
          params.combination_strategy?.segment_size_threshold,
        max_staleness: params.max_staleness,
      },
    },
    queryOptions,
  );
};
