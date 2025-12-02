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

import { SemanticStatusType } from '@/common/styles/status_styles';
import {
  generateTestInvestigateUrl,
  generateTestInvestigateUrlForLegacyInvocations,
} from '@/common/tools/url_utils';
import {
  StringPair,
  GerritChange,
  Variant,
  Sources,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { TestLocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_metadata.pb';
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import {
  TestVariant,
  TestVariantStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  AnyInvocation,
  isRootInvocation,
} from '@/test_investigation/utils/invocation_utils';

export interface FormattedCLInfo {
  display: string;
  url: string;
  key: string;
}

export interface RateBoxDisplayInfo {
  text: string;
  statusType: SemanticStatusType;
  ariaLabel: string;
}

export interface SegmentPoint {
  rateBox: RateBoxDisplayInfo;
  isContextual: boolean;
  startPosition: number;
  endPosition: number;
  startHour?: string;
}

export interface DisplayableHistorySegments {
  displayedSegmentPoints: SegmentPoint[];
  hasMoreNewer?: boolean;
  hasMoreOlder?: boolean;
}

export interface HistoryAnalysis {
  analysisText: string;
  latestPostsubmitRateBox?: RateBoxDisplayInfo;
  itemStatus?: SemanticStatusType;
}

/**
 * Query params used to navigate the Android Test Hub page.
 */
interface AndroidTestHubQueryParams {
  configPath?: string | null;
  query?: string | null;
  tab?: string | null;
  clusterId?: string | null;
  filters?: string[] | null;
}

/** The host for the Android Build Corp server. */
export const ANDROID_BUILD_CORP_HOST = 'https://android-build.corp.google.com';

export function getInvocationTag(
  tags: readonly StringPair[] | undefined,
  key: string,
): string | undefined {
  return tags?.find((tag) => tag.key === key)?.value;
}

export function getCommitInfoFromInvocation(
  invocation: AnyInvocation | null,
): string | undefined {
  if (!invocation) return undefined;

  const gc = getSourcesFromInvocation(invocation)?.gitilesCommit;

  if (gc) {
    let str =
      gc.ref?.replace(/^refs\/heads\//, '').replace(/^refs\/tags\//, 'tag:') ||
      '';
    if (gc.commitHash) {
      str += `${str ? '@' : ''}${gc.commitHash.substring(0, 8)}`;
    }
    if (gc.position) {
      str += ` (#${gc.position})`;
    }
    return str || undefined;
  }

  const commitHashFromTag = getInvocationTag(
    invocation.tags,
    'git.commit.hash',
  );
  if (commitHashFromTag) {
    return `${commitHashFromTag.substring(0, 8)} (from tag)`;
  }
  return undefined;
}

/**
 * Constructs a Gitiles commit URL for the base commit from an Invocation object.
 *
 * @param invocation The invocation object, which may contain Gitiles source info.
 * @returns A fully-formed Gitiles URL string, or undefined if essential information is missing.
 */
export function getCommitGitilesUrlFromInvocation(
  invocation: AnyInvocation,
): string | undefined {
  const gc = getSourcesFromInvocation(invocation)?.gitilesCommit;

  if (gc && gc.host && gc.project && gc.commitHash) {
    return `https://${gc.host}/${gc.project}/+/${gc.commitHash}`;
  }
  return undefined;
}

export function getVariantValue(
  variantDef: Variant | undefined,
  key: string,
): string | undefined {
  return variantDef?.def?.[key];
}

export function formatAllCLs(
  gerritChanges: readonly GerritChange[] | undefined,
): FormattedCLInfo[] {
  if (!gerritChanges?.length) return [];
  return gerritChanges
    .map((cl, i) => {
      if (!cl.host || !cl.project || !cl.change) return null;
      const pPath = cl.project.startsWith('/') ? cl.project : `/${cl.project}`;
      let url = `https://${cl.host}/c${pPath}/+/${cl.change}`;
      let disp = `CL ${cl.change}`;
      if (cl.patchset && cl.patchset !== '0') {
        url += `/${cl.patchset}`;
        disp += `/${cl.patchset}`;
      }
      return {
        display: disp,
        url,
        key: `${cl.host}-${cl.project}-${cl.change}-${cl.patchset || i}`,
      };
    })
    .filter((clInfo): clInfo is FormattedCLInfo => clInfo !== null);
}

export function constructTestHistoryUrl(
  project?: string,
  testId?: string,
  variantDef?: { readonly [key: string]: string } | undefined,
): string {
  if (!project || !testId) {
    return '#error-missing-project-or-testid';
  }

  const encodedTestId = encodeURIComponent(testId);
  let url = `/ui/test/${project}/${encodedTestId}`;

  if (variantDef) {
    const variantQueryParts: string[] = [];
    for (const [key, value] of Object.entries(variantDef)) {
      if (value !== undefined && value !== null) {
        variantQueryParts.push(`V:${key}=${encodeURIComponent(value)}`);
      }
    }

    if (variantQueryParts.length > 0) {
      const params = new URLSearchParams();
      params.set('q', variantQueryParts.join(' '));
      url += `?${params.toString()}`;
    }
  }
  return url;
}

export function constructFileBugUrl(
  invocation: AnyInvocation | null,
  testVariant: TestVariant | null,
  builderName?: string,
  hotlistId?: string,
): string {
  if (!invocation || !testVariant)
    return 'about:blank#error-missing-invocation-or-testvariant';
  const testDisplayName = testVariant.testMetadata?.name || testVariant.testId;
  let primaryFailureReason = 'N/A';
  if (testVariant.results) {
    for (const tvResultLink of testVariant.results) {
      if (tvResultLink.result?.statusV2 === TestResult_Status.FAILED) {
        primaryFailureReason =
          tvResultLink.result.failureReason?.primaryErrorMessage ||
          'Unknown failure reason';
        break;
      }
    }
    if (
      primaryFailureReason === 'N/A' &&
      testVariant.status === TestVariantStatus.UNEXPECTED &&
      testVariant.results[0]?.result?.failureReason
    ) {
      primaryFailureReason =
        testVariant.results[0].result.failureReason.primaryErrorMessage ||
        'Unknown failure reason';
    }
  }
  const title = `Failure in ${testDisplayName} on ${builderName || 'unknown builder'}`;
  let buildUiLink = `[Build resource: ${invocation.producerResource || 'N/A'}]`;
  const bbBuildId = getBuildBucketBuildId(invocation);
  if (bbBuildId) {
    buildUiLink = bbBuildId
      ? `https://ci.chromium.org/b/${bbBuildId}`
      : buildUiLink;
  }
  const currentUrl =
    typeof window !== 'undefined' && window.location.href !== 'about:blank'
      ? window.location.href
      : '[Current Page URL]';
  const description = `Test: ${testDisplayName}
Variant: ${JSON.stringify(testVariant.variant?.def || {})}
Failure Reason: ${primaryFailureReason}

Invocation: ${invocation.name}
Build Link: ${buildUiLink}
Realm: ${invocation.realm || 'N/A'}
Test Investigation Page: ${currentUrl}
(Auto-generated bug from Test Investigation Page)\n`;
  const params = new URLSearchParams();
  params.append('title', title);
  params.append('description', description);
  const componentId =
    testVariant.testMetadata?.bugComponent?.issueTracker?.componentId;
  if (componentId) params.append('component', componentId.toString());
  if (hotlistId && hotlistId !== '[HOTLIST_ID_PLACEHOLDER]')
    params.append('hotlistIds', hotlistId);
  return `https://b.corp.google.com/issues/new?${params.toString()}`;
}

export function constructCodesearchUrl(
  location?: TestLocation,
): string | undefined {
  if (!location?.fileName) {
    return undefined;
  }
  const filePath = location.fileName.startsWith('//')
    ? location.fileName.substring(2)
    : location.fileName;
  const params = new URLSearchParams();
  let query = `file:${filePath}`;
  if (location.line && location.line > 0) query += `:${location.line}`;
  params.append('q', query);
  return `https://source.corp.google.com/search?${params.toString()}`;
}

/**
 * Checks if the current invocation is for a presubmit (CL) run.
 */
export function isPresubmitRun(invocation: AnyInvocation | null): boolean {
  if (!invocation) return false;

  return (getSourcesFromInvocation(invocation)?.changelists?.length || 0) > 0;
}

/**
 * Extracts a BuildBucket build ID from an invocation.
 * It robustly checks the producerResource (which exists on both legacy
 * and root invocations) and falls back to the legacy name format.
 */
export function getBuildBucketBuildId(
  invocation: AnyInvocation | null,
): string | undefined {
  if (!invocation) {
    return undefined;
  }

  const resource = invocation.producerResource;

  if (resource) {
    let name = '';
    if (typeof resource !== 'string') {
      name = resource.name;
    } else {
      name = resource;
    }
    const match = name.match(/\/builds\/(\d+)$/);
    if (match) {
      return match[1];
    }
    return undefined;
  }

  // Fallback to checking the legacy name format.
  if (
    !isRootInvocation(invocation) &&
    invocation.name.startsWith('invocations/build-')
  ) {
    return invocation.name.substring('invocations/build-'.length);
  }

  return undefined;
}

/**
 * A helper to robustly get the Sources object from either a
 * RootInvocation or a legacy Invocation.
 */
export function getSourcesFromInvocation(
  invocation: AnyInvocation | null,
): Sources | null | undefined {
  if (!invocation) {
    return null;
  }
  if (isRootInvocation(invocation)) {
    return invocation.sources;
  }
  // It must be a legacy Invocation
  return invocation.sourceSpec?.sources;
}

/**
 * Generates the correct URL to a test investigation page, deciding whether
 * to use the legacy or structured format based on the invocation mode.
 */
export function getTestVariantURL(
  invocationId: string,
  testVariant: TestVariant,
  isLegacyInvocation: boolean,
): string {
  // If the invocation is legacy, always generate the legacy-structured URL.
  if (isLegacyInvocation) {
    return generateTestInvestigateUrlForLegacyInvocations(
      invocationId,
      testVariant.testId,
      testVariant.variantHash,
    );
  }

  // Otherwise, we are on a RootInvocation.
  // Prefer the fully-structured URL if the structured ID is available.
  if (testVariant.testIdStructured) {
    return generateTestInvestigateUrl(
      invocationId,
      testVariant.testIdStructured,
    );
  }

  // Fallback for root invocations that might have non-structured test variants.
  return generateTestInvestigateUrlForLegacyInvocations(
    invocationId,
    testVariant.testId,
    testVariant.variantHash,
  );
}

export function isAnTSInvocation(invocation: AnyInvocation) {
  return invocation.realm?.startsWith('android:') || false;
}

/**
 * Returns the full method name including the package and class.
 */
export function getFullMethodName(testVariant: TestVariant): string {
  const testCaseName = testVariant?.testIdStructured?.caseName ?? '';
  if (testCaseName === '') {
    return '';
  }
  const className = testVariant?.testIdStructured?.coarseName ?? '';
  const packageName = testVariant?.testIdStructured?.fineName ?? '';
  return `${className}.${packageName}#${testCaseName}`;
}

/**
 * Generate url link to Android Test Hub.
 */
export function getAndroidTestHubUrl(inputParams: AndroidTestHubQueryParams) {
  const params = new URLSearchParams();
  if (inputParams.configPath) {
    params.append('config', String(inputParams.configPath));
  }
  if (inputParams.query) {
    params.append('query', String(inputParams.query));
  }
  if (inputParams.tab) {
    params.append('tab', String(inputParams.tab));
  }
  if (inputParams.clusterId) {
    params.append('clusterId', String(inputParams.clusterId));
  }
  if (inputParams.filters) {
    for (const filter of inputParams.filters) {
      params.append('filter', filter);
    }
  }

  return `${ANDROID_BUILD_CORP_HOST}/builds/tests/search?${params.toString()}`;
}

export function getBuildDetailsUrl(buildId: string, target: string) {
  return (
    `${ANDROID_BUILD_CORP_HOST}` +
    `/builds/build-details/${encodeURIComponent(buildId)}/targets/${encodeURIComponent(target)}`
  );
}
