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

import { OutputCommit } from '@/gitiles/types';
import {
  Changepoint,
  ChangepointGroupSummary,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import {
  ClusterResponse,
  ClusterResponse_ClusteredTestResult,
  ClusterResponse_ClusteredTestResult_ClusterEntry,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { Variant } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import { SourceRef } from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';
import {
  BatchGetTestVariantBranchResponse,
  QuerySourcePositionsResponse,
  Segment,
  SourcePosition,
  TestVariantBranch,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import {
  TestVerdict,
  TestVerdictStatus,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';

/**
 * @fileoverview
 * Declare a list of types that are the same as the generated protobuf types
 * except that we also bake in the knowledge of which properties are always
 * defined when they are queried from the RPCs. This allows us to limit type
 * casting to only where the data is queried instead of having to use non-null
 * coercion (i.e. `object.nullable!`) everywhere.
 */

export type SpecifiedTestVerdictStatus = Exclude<
  TestVerdictStatus,
  TestVerdictStatus.UNSPECIFIED
>;

export interface OutputTestVerdict extends TestVerdict {
  readonly status: SpecifiedTestVerdictStatus;
}

export type OutputClusterEntry = NonNullableProps<
  ClusterResponse_ClusteredTestResult_ClusterEntry,
  'clusterId'
>;

export interface OutputClusteredTestResult
  extends ClusterResponse_ClusteredTestResult {
  readonly clusters: readonly OutputClusterEntry[];
}

export interface OutputClusterResponse extends ClusterResponse {
  readonly clusteredTestResults: readonly OutputClusteredTestResult[];
}

export type OutputChangepointGroupSummary = DeepNonNullableProps<
  ChangepointGroupSummary,
  'canonicalChangepoint' | 'statistics'
>;

export type OutputSegment = NonNullableProps<
  Segment,
  'counts' | 'startHour' | 'endHour'
>;

export interface OutputTestVariantBranch extends TestVariantBranch {
  readonly segments: readonly OutputSegment[];
  readonly ref: NonNullableProps<SourceRef, 'gitiles'>;
}

export interface OutputBatchGetTestVariantBranchResponse
  extends BatchGetTestVariantBranchResponse {
  readonly testVariantBranches: readonly OutputTestVariantBranch[];
}

export interface OutputSourcePosition extends SourcePosition {
  readonly commit: OutputCommit;
  readonly verdicts: readonly OutputTestVerdict[];
}

export interface OutputQuerySourcePositionsResponse
  extends QuerySourcePositionsResponse {
  readonly sourcePositions: readonly OutputSourcePosition[];
}

export type OutputChangepoint = NonNullableProps<Changepoint, 'variant'>;

export interface ParsedTestVariantBranchName {
  readonly project: string;
  readonly testId: string;
  readonly variantHash: string;
  readonly refHash: string;
}

const TEST_VARIANT_BRANCH_NAME_RE =
  /^projects\/(?<project>[^/]+)\/tests\/(?<urlEscapedTestId>[^/]+)\/variants\/(?<variantHash>[^/]+)\/refs\/(?<refHash>[^/]+)$/;

export const ParsedTestVariantBranchName = {
  fromString(name: string): ParsedTestVariantBranchName {
    const match = TEST_VARIANT_BRANCH_NAME_RE.exec(name);
    if (!match) {
      throw new Error(`invalid TestVariantBranchName: ${name}`);
    }
    const { project, urlEscapedTestId, variantHash, refHash } =
      match.groups || {};

    return {
      project,
      testId: decodeURIComponent(urlEscapedTestId),
      variantHash,
      refHash,
    };
  },
  toString(name: ParsedTestVariantBranchName): string {
    const { project, testId, variantHash, refHash } = name;
    const urlEscapedTestId = encodeURIComponent(testId);
    return `projects/${project}/tests/${urlEscapedTestId}/variants/${variantHash}/refs/${refHash}`;
  },
};

export interface TestVariantBranchDef extends ParsedTestVariantBranchName {
  readonly variant: Variant | undefined;
}
