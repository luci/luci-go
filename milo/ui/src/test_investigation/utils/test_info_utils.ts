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
  StringPair,
  GerritChange,
  Variant,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestLocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_metadata.pb';
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import {
  TestVariant,
  TestVariantStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

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

export function getInvocationTag(
  tags: readonly StringPair[] | undefined,
  key: string,
): string | undefined {
  return tags?.find((tag) => tag.key === key)?.value;
}

export function getCommitInfoFromInvocation(
  invocation: Invocation | null,
): string | undefined {
  if (!invocation) return undefined;
  const gc = invocation.sourceSpec?.sources?.gitilesCommit;
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
        variantQueryParts.push(`V:${key}=${String(value)}`);
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
  invocation: Invocation | null,
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
  if (
    invocation.producerResource &&
    invocation.producerResource.includes('cr-buildbucket')
  ) {
    const bbBuildId = invocation.producerResource.split('/').pop();
    buildUiLink = bbBuildId
      ? `https://ci.chromium.org/b/${bbBuildId}`
      : buildUiLink;
  }
  const currentUrl =
    typeof window !== 'undefined' && window.location.href !== 'about:blank'
      ? window.location.href
      : '[Current Page URL]';
  // Note that whitespace is significant in this string.
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
  if (!location?.fileName) return undefined;
  const filePath = location.fileName.startsWith('//')
    ? location.fileName.substring(2)
    : location.fileName;
  const params = new URLSearchParams();
  let query = `file:${filePath}`;
  if (location.repo) {
    try {
      if (location.repo.includes('://')) {
        const repoUrl = new URL(location.repo);
        const repoPath = repoUrl.pathname
          .replace(/^\/\+\//, '')
          .replace(/^\//, '');
        if (repoPath) query += ` repo:${repoPath}`;
      } else if (location.repo.trim() !== '') {
        query += ` repo:${location.repo.trim()}`;
      }
    } catch (e) {
      if (location.repo.trim() !== '') query += ` repo:${location.repo.trim()}`;
    }
  }
  if (location.line && location.line > 0) query += ` line:${location.line}`;
  params.append('q', query);
  return `https://source.corp.google.com/search?${params.toString()}`;
}
