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
import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import {
  StringPair,
  GerritChange,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestLocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_metadata.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { Timestamp } from '@/proto/google/protobuf/timestamp.pb';

// Core data shapes defined and exported by these utils
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

export interface SegmentAnalysisResult {
  currentRateBox?: RateBoxDisplayInfo;
  previousRateBox?: RateBoxDisplayInfo;
  transitionText?: string;
  stabilitySinceText?: string;
}

// Utility functions
export function getInvocationTag(
  tags: readonly StringPair[] | undefined,
  key: string,
): string | undefined {
  return tags?.find((tag) => tag.key === key)?.value;
}

export function getCommitInfoFromInvocation(
  invocation: Invocation,
): string | undefined {
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
  variantDef: TestVariant['variant'],
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

export function formatDurationSince(
  ts?: Timestamp | string,
): string | undefined {
  let dateInMillis: number | undefined;

  if (!ts) {
    return undefined;
  }

  if (typeof ts === 'string') {
    const parsedDate = new Date(ts);
    if (!isNaN(parsedDate.getTime())) {
      dateInMillis = parsedDate.getTime();
    }
  } else {
    // Assume ts is a Timestamp object (or structurally similar)
    if (ts.seconds !== null && ts.seconds !== undefined) {
      const secs =
        typeof ts.seconds === 'bigint'
          ? Number(ts.seconds)
          : Number(ts.seconds);
      const nanos = Number(ts.nanos || 0);
      dateInMillis = secs * 1000 + nanos / 1000000;
    }
  }

  if (dateInMillis === undefined || isNaN(dateInMillis)) {
    return undefined;
  }

  const now = new Date();
  const diffMs = now.getTime() - dateInMillis;

  if (diffMs < 0) {
    return 'just now';
  }

  const diffSeconds = Math.floor(diffMs / 1000);
  const diffMinutes = Math.floor(diffSeconds / 60);
  const diffHours = Math.floor(diffMinutes / 60);

  if (diffHours < 1) {
    if (diffMinutes < 1) {
      return 'just now';
    }
    return `${diffMinutes} minute${diffMinutes !== 1 ? 's' : ''} ago`;
  }
  if (diffHours < 24) {
    return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
  }

  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;
}

export function determineRateBoxStatusType(
  ratePercent: number,
): SemanticStatusType {
  if (ratePercent < 5) return 'success';
  if (ratePercent > 95) return 'error';
  return 'warning';
}

// TODO: Much of this logic feels very linked to the presentation and should be moved to the appropriate components.
export function analyzeSegments(
  segments?: readonly Segment[],
): SegmentAnalysisResult {
  if (!segments || segments.length === 0) {
    return {
      transitionText: 'No segment data available for transition analysis.',
    };
  }

  let currentRateBox: RateBoxDisplayInfo = {
    text: 'N/A',
    statusType: 'info',
    ariaLabel: 'Current failure rate not available',
  };
  let previousRateBox: RateBoxDisplayInfo | undefined;
  let transitionText: string | undefined;
  const currentSegment = segments[0];

  if (currentSegment.counts) {
    const { unexpectedResults = 0, totalResults = 0 } = currentSegment.counts;
    const currentRatePercent =
      totalResults > 0 ? (unexpectedResults / totalResults) * 100 : 0;
    const statusType = determineRateBoxStatusType(currentRatePercent);
    currentRateBox = {
      text: `${currentRatePercent.toFixed(0)}%`,
      statusType: statusType,
      ariaLabel: `Current failure rate ${currentRatePercent.toFixed(0)}%`,
    };
  }

  const stabilitySinceText = formatDurationSince(currentSegment.startHour);

  if (
    currentSegment.hasStartChangepoint &&
    segments.length > 1 &&
    currentRateBox.text !== 'N/A'
  ) {
    const prevSegment = segments[1];
    if (prevSegment.counts) {
      const {
        unexpectedResults: prevUnexpected = 0,
        totalResults: prevTotal = 0,
      } = prevSegment.counts;
      const prevRatePercent =
        prevTotal > 0 ? (prevUnexpected / prevTotal) * 100 : 0;
      const prevStatusType = determineRateBoxStatusType(prevRatePercent);
      previousRateBox = {
        text: `${prevRatePercent.toFixed(0)}%`,
        statusType: prevStatusType,
        ariaLabel: `Previous failure rate ${prevRatePercent.toFixed(0)}%`,
      };
      transitionText = `The test recently transitioned from ~${prevRatePercent.toFixed(0)}%
                        to ~${currentRateBox.text} failure rate ${stabilitySinceText || 'recently'}.`;
    } else {
      transitionText = `The test started failing at ~${currentRateBox.text} failure rate
                        ${stabilitySinceText || 'recently'} (previously no results in segment).`;
    }
  } else if (currentRateBox?.text !== 'N/A') {
    transitionText = `The test has been at ~${currentRateBox.text} failure rate
                      ${stabilitySinceText ? `for ${stabilitySinceText}` : 'for some time'}.`;
  } else if (
    !currentSegment.counts &&
    segments.length > 1 &&
    segments[1].counts
  ) {
    transitionText = `No recent data for this test. ${stabilitySinceText ? `Last data point was ${stabilitySinceText}.` : ''}`;
  }

  return {
    currentRateBox,
    previousRateBox,
    transitionText,
    stabilitySinceText,
  };
}

export function constructFileBugUrl(
  invocation: Invocation,
  testVariant: TestVariant,
  builderName?: string,
  hotlistId?: string,
): string {
  const testDisplayName = testVariant.testMetadata?.name || testVariant.testId;
  const primaryFailureReason =
    testVariant.results?.[0]?.result?.failureReason?.primaryErrorMessage ||
    'N/A';
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

  const description = `Test: ${testDisplayName}
Variant: ${JSON.stringify(testVariant.variant?.def || {})}
Failure Reason: ${primaryFailureReason}

Invocation: ${invocation.name}
Build Link: ${buildUiLink}
Realm: ${invocation.realm || 'N/A'}

Test Investigation Page: ${currentUrl}

(Auto-generated bug from Test Investigation Page)
`;
  const params = new URLSearchParams();
  params.append('title', title);
  params.append('description', description);
  const componentId =
    testVariant.testMetadata?.bugComponent?.issueTracker?.componentId;
  if (componentId) {
    params.append('component', componentId.toString());
  }
  if (hotlistId && hotlistId !== '[HOTLIST_ID_PLACEHOLDER]') {
    params.append('hotlistIds', hotlistId);
  }
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

  if (location.repo) {
    try {
      const repoUrl = new URL(location.repo);
      // Basic parsing for common Gitiles URL patterns like https://host/project/+/ref
      // or https://host/project (where project might be 'chromium/src')
      const repoPath = repoUrl.pathname
        .replace(/^\/\+\//, '')
        .replace(/^\//, ''); // Remove leading /+/ or /
      if (repoPath) {
        query += ` repo:${repoPath}`;
      }
    } catch (e) {
      // If repo is not a valid URL or parsing fails, proceed without repo context
    }
  }

  if (location.line && location.line > 0) {
    query += ` line:${location.line}`;
  }
  params.append('q', query);
  return `https://source.corp.google.com/search?${params.toString()}`;
}
