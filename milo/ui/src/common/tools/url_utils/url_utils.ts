// Copyright 2022 The LUCI Authors.
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

import { Build, BuilderID } from '@/common/services/buildbucket';
import {
  TestIdentifier,
  Variant,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { TestLocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_metadata.pb';

export function getBuildURLPathFromBuildData(
  build: Pick<Build, 'builder' | 'number' | 'id'>,
): string {
  return getBuildURLPath(
    build.builder,
    build.number ? build.number.toString() : `b${build.id}`,
  );
}

export function getBuildURLPathFromBuildId(buildId: string): string {
  return `/ui/b/${buildId}`;
}

export function getBuildURLPath(
  builder: BuilderID,
  buildIdOrNum: string,
): string {
  return `${getBuilderURLPath(builder)}/${buildIdOrNum}`;
}

export function getBuilderURLPath(builder: BuilderID): string {
  return `/ui/p/${builder.project}/builders/${
    builder.bucket
  }/${encodeURIComponent(builder.builder)}`;
}

export function getOldConsoleURLPath(proj: string, consoleId: string) {
  return `/p/${proj}/g/${encodeURIComponent(consoleId)}/console`;
}

export function getProjectURLPath(proj: string) {
  return `/ui/p/${proj}`;
}

export function getLegacyBuildURLPath(
  builder: BuilderID,
  buildNumOrId: string,
) {
  return `/old/p/${builder.project}/builders/${
    builder.bucket
  }/${encodeURIComponent(builder.builder)}/${buildNumOrId}`;
}

export function getSwarmingTaskURL(hostname: string, taskId: string): string {
  return `https://${hostname}/task?id=${taskId}&o=true&w=true`;
}

export function getSwarmingBotListURL(
  hostname: string,
  dimensions: readonly string[],
): string {
  return `https://${hostname}/botlist?${new URLSearchParams(
    dimensions.map((d) => ['f', d]),
  )}`;
}

export function getInvURLPath(invId: string): string {
  return `/ui/inv/${invId}`;
}

export function getRawArtifactURLPath(artifactName: string): string {
  return `/raw-artifact/${artifactName}`;
}

export function getImageDiffArtifactURLPath(
  diffArtifactName: string,
  expectedArtifactId: string,
  actualArtifactId: string,
) {
  const search = new URLSearchParams();
  search.set('actual_artifact_id', actualArtifactId);
  search.set('expected_artifact_id', expectedArtifactId);
  return `/ui/artifact/image-diff/${diffArtifactName}?${search}`;
}

export function getTextDiffArtifactURLPath(artifactName: string) {
  return `/ui/artifact/text-diff/${artifactName}`;
}

export function getTestHistoryURLPath(project: string, testId: string) {
  return `/ui/test/${project}/${encodeURIComponent(testId)}`;
}

export function generateTestHistoryURLSearchParams(variant: Variant) {
  return Object.entries(variant.def || {})
    .map(
      ([dimension, value]) =>
        `V:${encodeURIComponent(dimension)}=${encodeURIComponent(value)}`,
    )
    .join(' ');
}

export function getTestHistoryURLWithSearchParam(
  project: string,
  testId: string,
  queryParam: string,
  redirected?: boolean,
) {
  let result = getTestHistoryURLPath(project, testId);
  const params = [];
  if (queryParam) {
    params.push(['q', queryParam]);
  }
  if (redirected) {
    params.push(['redirected', 'true']);
  }
  if (params.length > 0) {
    result += `?${new URLSearchParams(params).toString()}`;
  }
  return result;
}

export const NOT_FOUND_URL = '/ui/not-found';

export function getLoginUrl(redirectTo: string) {
  return `/auth/openid/login?${new URLSearchParams({ r: redirectTo })}`;
}

export function getLogoutUrl(redirectTo: string) {
  return `/auth/openid/logout?${new URLSearchParams({ r: redirectTo })}`;
}

export function getCodeSourceUrl(testLocation: TestLocation, branch = 'HEAD') {
  return (
    testLocation.repo +
    '/+/' +
    branch +
    testLocation.fileName.slice(1) +
    (testLocation.line ? '#' + testLocation.line : '')
  );
}

/**
 * Update a single query parameter and return the update query string.
 *
 * @param currentSearchString The current search parameters string.
 * @param paramKey The parameter key to set.
 * @param paramValue The value to set.
 * @returns The updated parameter string.
 */
export function setSingleQueryParam(
  currentSearchString: string,
  paramKey: string,
  paramValue: string,
): URLSearchParams {
  const updatedSearchParams = new URLSearchParams(currentSearchString);
  updatedSearchParams.set(paramKey, paramValue);
  return updatedSearchParams;
}

/**
 * Returns a link to the AuthDB group page for the given group.
 *
 * @param groupName Name of the group.
 * @param currentTab Optional arg specifying the selected tab when loading
 * group details. If not specified, the default tab set by the group details
 * page will be selected instead.
 * @returns relative path to the group's details page.
 */
export function getAuthGroupURLPath(
  groupName: string,
  currentTab?: string | null,
) {
  const target = `/ui/auth/groups/${encodeURIComponent(groupName)}`;
  if (!currentTab) {
    return target;
  }
  return `${target}?tab=${currentTab}`;
}

/**
 * Returns a link to the AuthDB lookup page for the given principal.
 *
 * @param principal Principal to be searched, e.g. a@example.com, service:*.
 * @param currentTab Optional arg specifying the selected tab when loading
 * group details. If not specified, the default tab set by the lookup page will
 * be selected instead.
 * @returns relative path to the lookup page for the principal.
 */
export function getAuthLookupURLPath(
  principal: string,
  currentTab?: string | null,
) {
  const target = `/ui/auth/lookup?p=${encodeURIComponent(principal)}`;
  if (!currentTab) {
    return target;
  }
  return `${target}&tab=${currentTab}`;
}

/**
 * Regex to extract the old URL values.
 */
const LEGACY_URL_REGEX =
  /^\/ui\/test-investigate\/invocations\/([^/]+)\/tests\/([^/]+)\/variants\/([^/?#]+)/;

/**
 * Generates the new test investigate URL from a legacy URL.
 * Assumes all old URLs are legacy and maps to the new legacy-compatible format.
 *
 * @param oldUrl The old URL path (e.g., /ui/test-investigate/invocations/inv123/tests/test.name/variants/abc)
 * @returns The new URL path, or the original path if it doesn't match.
 */
export function generateTestInvestigateUrlFromOld(oldUrl: string): string {
  const match = oldUrl.match(LEGACY_URL_REGEX);

  if (!match) {
    return oldUrl; // Not a match, return original
  }

  const [, invocationId, encodedTestId, variantHash] = match;

  const newPath =
    `/ui/test-investigate/invocations/${invocationId}` +
    `/modules/legacy/schemes/legacy/variants/${variantHash}/cases/${encodedTestId}`;

  // This captures the query and hash, e.g., "?foo=bar#hash"
  const queryHashPart = oldUrl.substring(match[0].length);
  const existingQuery = queryHashPart.match(/^\?([^#]+)/);
  const existingHash = queryHashPart.match(/#(.*)$/);

  let finalUrl = `${newPath}`;

  if (existingQuery) {
    finalUrl += `&${existingQuery[1]}`; // Append existing queries
  }
  if (existingHash) {
    finalUrl += existingHash[0];
  }

  return finalUrl;
}

/**
 * Generates the new, structured URL for a "legacy" test,
 * given the core components from the old URL format.
 * This correctly URL-encodes the testId and adds the legacy query param.
 */
export function generateTestInvestigateUrlForLegacyInvocations(
  invocationId: string,
  testId: string,
  variantHash: string,
): string {
  const encodedTestId = encodeURIComponent(testId);
  return (
    `/ui/test-investigate/invocations/${invocationId}/` +
    `modules/legacy/schemes/legacy/variants/${variantHash}/cases/${encodedTestId}`
  );
}

/**
 * Generates the correct test investigation URL (either structured or legacy)
 * from an invocation ID and a structured TestIdentifier proto.
 *
 * This intelligently detects legacy tests and routes to the correct generator.
 */
export function generateTestInvestigateUrl(
  invocationId: string,
  testId: TestIdentifier,
): string {
  // Check for the special legacy case first, as defined in the proto.
  if (testId.moduleName === 'legacy' && testId.moduleScheme === 'legacy') {
    return generateTestInvestigateUrlForLegacyInvocations(
      invocationId,
      testId.caseName, // The full legacy ID is stored in caseName
      testId.moduleVariantHash, // The variant hash
    );
  }

  // It's a structured URL.
  let path =
    `/ui/test-investigate/invocations/${invocationId}` +
    `/modules/${encodeURIComponent(testId.moduleName)}/schemes/${testId.moduleScheme}/variants/${testId.moduleVariantHash}`;

  // Append optional segments in the correct order
  if (testId.coarseName) {
    path += `/coarse/${encodeURIComponent(testId.coarseName)}`;
  }
  if (testId.fineName) {
    path += `/fine/${encodeURIComponent(testId.fineName)}`;
  }

  // Append the required case, which must be encoded
  path += `/cases/${encodeURIComponent(testId.caseName)}`;

  // This is the non-legacy URL, so no  is added.
  return path;
}
