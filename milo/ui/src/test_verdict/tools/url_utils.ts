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

import { ParsedTestVariantBranchName } from '@/analysis/types';
import {
  Changepoint,
  ChangepointPredicate,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { TestVariantBranch } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

export interface GetRegressionDetailsUrlPathOpts {
  readonly canonicalChangepoint: Changepoint;
  readonly predicate?: ChangepointPredicate;
}

export function getRegressionDetailsURLPath({
  canonicalChangepoint,
  predicate,
}: GetRegressionDetailsUrlPathOpts) {
  const project = canonicalChangepoint.project;
  const searchParams = new URLSearchParams({
    tvb: ParsedTestVariantBranchName.toString(canonicalChangepoint),
    sh: canonicalChangepoint.startHour!,
    nsp: canonicalChangepoint.nominalStartPosition,
    ...(predicate
      ? { cp: JSON.stringify(ChangepointPredicate.toJSON(predicate)) }
      : {}),
  });

  return `/ui/labs/p/${project}/regressions/details?${searchParams}`;
}

export function getBlamelistUrl(
  tvb: ParsedTestVariantBranchName,
  commitPosition?: string,
) {
  const urlBase =
    `/ui/labs/p/${encodeURIComponent(tvb.project)}` +
    `/tests/${encodeURIComponent(tvb.testId)}` +
    `/variants/${tvb.variantHash}` +
    `/refs/${tvb.refHash}` +
    `/blamelist`;
  const hash = commitPosition ? `#CP-${commitPosition}` : '';

  return urlBase + hash;
}

/**
 * Converts a TestVariantBranch into a ParsedTestVariantBranchName.
 * Returns undefined if tvb is missing or lacks required fields (refHash, project, testId).
 */
export function toParsedTestVariantBranchName(
  tvb?: TestVariantBranch | null,
): ParsedTestVariantBranchName | undefined {
  if (!tvb?.refHash || !tvb.project || !tvb.testId) {
    return undefined;
  }
  return {
    project: tvb.project,
    testId: tvb.testId,
    variantHash: tvb.variantHash || '',
    refHash: tvb.refHash,
  };
}

/**
 * Returns a blamelist URL given a TestVariantBranch and an optional commit position.
 * Returns undefined if tvb is missing or lacks required fields (refHash, project, testId).
 */
export function getBlamelistUrlFromVariantBranch(
  tvb?: TestVariantBranch | null,
  commitPosition?: string,
): string | undefined {
  const parsed = toParsedTestVariantBranchName(tvb);
  return parsed ? getBlamelistUrl(parsed, commitPosition) : undefined;
}
