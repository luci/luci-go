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

import { Build, BuilderID, GerritChange } from '@/common/services/buildbucket';
import { TestLocation } from '@/common/services/resultdb';

export function getBuildURLPathFromBuildData(
  build: Pick<Build, 'builder' | 'number' | 'id'>
): string {
  return getBuildURLPath(
    build.builder,
    build.number ? build.number.toString() : `b${build.id}`
  );
}

export function getBuildURLPathFromBuildId(buildId: string): string {
  return `/ui/b/${buildId}`;
}

export function getBuildURLPath(
  builder: BuilderID,
  buildIdOrNum: string
): string {
  return `/ui${getBuilderURLPath(builder)}/${buildIdOrNum}`;
}

export function getBuilderURLPath(builder: BuilderID): string {
  return `${getProjectURLPath(builder.project)}/builders/${
    builder.bucket
  }/${encodeURIComponent(builder.builder)}`;
}

export function getProjectURLPath(proj: string): string {
  return `/p/${proj}`;
}

export function getLegacyBuildURLPath(
  builder: BuilderID,
  buildNumOrId: string
) {
  return `${getLegacyBuilderURLPath(builder)}/${buildNumOrId}`;
}

export function getLegacyBuilderURLPath(builder: BuilderID) {
  return `/old${getBuilderURLPath(builder)}`;
}

export function getGerritChangeURL(change: GerritChange): string {
  return `https://${change.host}/c/${change.change}/${change.patchset}`;
}

export function getSwarmingTaskURL(hostname: string, taskId: string): string {
  return `https://${hostname}/task?id=${taskId}&o=true&w=true`;
}

export function getSwarmingBotListURL(
  hostname: string,
  dimensions: readonly string[]
): string {
  return `https://${hostname}/botlist?${new URLSearchParams(
    dimensions.map((d) => ['f', d])
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
  actualArtifactId: string
) {
  const search = new URLSearchParams();
  search.set('actual_artifact_id', actualArtifactId);
  search.set('expected_artifact_id', expectedArtifactId);
  return `/ui/artifact/image-diff/${diffArtifactName}?${search}`;
}

export function getTextDiffArtifactURLPath(artifactName: string) {
  return `/ui/artifact/text-diff/${artifactName}`;
}

export function getTestHistoryURLPath(realm: string, testId: string) {
  return `/ui/test/${realm}/${encodeURIComponent(testId)}`;
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
