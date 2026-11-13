// Copyright 2026 The LUCI Authors.
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

import { AB_CL_RANGE_BASE_URL } from '@/crystal_ball/constants';

/**
 * Options for building an Android Build CL range search URL.
 */
export interface AbClRangeUrlOptions {
  /** The newer build ID of the range. */
  fromBuildId: string;
  /** Whether the CLs of the `fromBuildId` build are included. */
  includeFrom: boolean;
  /** Whether the CLs of the `toBuildId` build are included. */
  includeTo: boolean;
  /** Optional build target to preselect on the range search page. */
  target?: string;
  /** The older build ID of the range. */
  toBuildId: string;
}

/**
 * Builds a URL to the Android Build CL range search page.
 */
export function buildAbClRangeUrl({
  fromBuildId,
  includeFrom,
  includeTo,
  target,
  toBuildId,
}: AbClRangeUrlOptions): string {
  const url = new URL(
    `${AB_CL_RANGE_BASE_URL}/from_id/${encodeURIComponent(fromBuildId)}` +
      `/to_id/${encodeURIComponent(toBuildId)}/`,
  );
  url.searchParams.set('includeFrom', includeFrom ? '1' : '0');
  url.searchParams.set('includeTo', includeTo ? '1' : '0');
  if (target) {
    url.searchParams.set('selectedFromTarget', target);
  }
  return url.toString();
}

/**
 * Builds a URL listing the CLs included in a single build. For presubmit
 * builds, this lists the CL(s) being tested.
 */
export function buildAbBuildClsUrl(buildId: string, target?: string): string {
  return buildAbClRangeUrl({
    fromBuildId: buildId,
    includeFrom: true,
    includeTo: true,
    target,
    toBuildId: buildId,
  });
}

/**
 * Builds a URL listing the CLs landed after `previousBuildId` up to and
 * including `buildId`.
 */
export function buildAbClsSinceUrl(
  previousBuildId: string,
  buildId: string,
  target?: string,
): string {
  return buildAbClRangeUrl({
    fromBuildId: buildId,
    includeFrom: true,
    includeTo: false,
    target,
    toBuildId: previousBuildId,
  });
}

/**
 * Returns whether the build ID is a sequential (postsubmit) build ID. Only
 * sequential build IDs form a meaningful CL range. Other builds (e.g.
 * presubmit builds) have a letter prefix such as `P12345`.
 */
export function isSequentialBuildId(buildId: string): boolean {
  return /^\d+$/.test(buildId);
}
