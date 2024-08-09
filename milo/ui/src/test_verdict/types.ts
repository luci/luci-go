// Copyright 2023 The LUCI Authors.
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

/**
 * @fileoverview
 * Declare a list of types that are the same as the generated protobuf types
 * except that we also bake in the knowledge of which properties are always
 * defined when they are queried from the RPCs. This allows us to limit type
 * casting to only where the data is queried instead of having to use non-null
 * coercion (i.e. `object.nullable!`) everywhere.
 */

import { SpecifiedTestVerdictStatus as AnalysisVerdictStatus } from '@/analysis/types';
import { NonNullableProps } from '@/generic_libs/types';
import { TestVerdictStatus } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import {
  BatchGetTestVariantsResponse,
  ArtifactMatchingContent,
  QueryTestVariantArtifactGroupsResponse_MatchGroup,
  QueryTestVariantArtifactGroupsResponse,
  QueryTestVariantArtifactsResponse,
  QueryInvocationVariantArtifactGroupsResponse_MatchGroup,
  QueryInvocationVariantArtifactGroupsResponse,
  QueryInvocationVariantArtifactsResponse,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import {
  TestResultBundle,
  TestVariant,
  TestVariantStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

export type SpecifiedTestVerdictStatus = Exclude<
  TestVariantStatus,
  | TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED
  | TestVariantStatus.UNEXPECTED_MASK
>;

const VERDICT_STATUS_MAP_FROM_ANALYSIS = Object.freeze({
  [TestVerdictStatus.UNEXPECTED]: TestVariantStatus.UNEXPECTED,
  [TestVerdictStatus.UNEXPECTEDLY_SKIPPED]:
    TestVariantStatus.UNEXPECTEDLY_SKIPPED,
  [TestVerdictStatus.FLAKY]: TestVariantStatus.FLAKY,
  [TestVerdictStatus.EXONERATED]: TestVariantStatus.EXONERATED,
  [TestVerdictStatus.EXPECTED]: TestVariantStatus.EXPECTED,
});

const VERDICT_STATUS_MAP_TO_ANALYSIS = Object.freeze({
  [TestVariantStatus.UNEXPECTED]: TestVerdictStatus.UNEXPECTED,
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]:
    TestVerdictStatus.UNEXPECTEDLY_SKIPPED,
  [TestVariantStatus.FLAKY]: TestVerdictStatus.FLAKY,
  [TestVariantStatus.EXONERATED]: TestVerdictStatus.EXONERATED,
  [TestVariantStatus.EXPECTED]: TestVerdictStatus.EXPECTED,
});

export const SpecifiedTestVerdictStatus = {
  fromAnalysis(status: AnalysisVerdictStatus): SpecifiedTestVerdictStatus {
    return VERDICT_STATUS_MAP_FROM_ANALYSIS[status];
  },
  toAnalysis(status: SpecifiedTestVerdictStatus): AnalysisVerdictStatus {
    return VERDICT_STATUS_MAP_TO_ANALYSIS[status];
  },
};

export type OutputTestResultBundle = NonNullableProps<
  TestResultBundle,
  'result'
>;

export interface OutputTestVerdict extends TestVariant {
  readonly status: SpecifiedTestVerdictStatus;
  readonly results: readonly OutputTestResultBundle[];
}

export interface OutputBatchGetTestVariantResponse
  extends BatchGetTestVariantsResponse {
  readonly testVariants: readonly OutputTestVerdict[];
}

export interface OutputQueryTestVariantArtifactGroupsResponse
  extends QueryTestVariantArtifactGroupsResponse {
  readonly groups: readonly OutputQueryTestVariantArtifactGroupsResponse_MatchGroup[];
}

export interface OutputQueryTestVariantArtifactsResponse
  extends QueryTestVariantArtifactsResponse {
  readonly artifacts: readonly OutputArtifactMatchingContent[];
}

export interface OutputQueryTestVariantArtifactGroupsResponse_MatchGroup
  extends QueryTestVariantArtifactGroupsResponse_MatchGroup {
  readonly artifacts: readonly OutputArtifactMatchingContent[];
}

export interface OutputQueryInvocationVariantArtifactGroupsResponse
  extends QueryInvocationVariantArtifactGroupsResponse {
  readonly groups: readonly OutputQueryInvocationVariantArtifactGroupsResponse_MatchGroup[];
}

export interface OutputQueryInvocationVariantArtifactsResponse
  extends QueryInvocationVariantArtifactsResponse {
  readonly artifacts: readonly OutputArtifactMatchingContent[];
}

export interface OutputQueryInvocationVariantArtifactGroupsResponse_MatchGroup
  extends QueryInvocationVariantArtifactGroupsResponse_MatchGroup {
  readonly artifacts: readonly OutputArtifactMatchingContent[];
}

export type OutputArtifactMatchingContent = NonNullableProps<
  ArtifactMatchingContent,
  'partitionTime'
>;
