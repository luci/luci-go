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

import { getBuildURLPathFromBuildId } from '@/common/tools/url_utils';

const BUILD_ID_TAG_RE = /^buildbucket_build_id:([1-9][0-9]+)$/;
const LOG_LOCATION_TAG_RE = /^log_location:(logdog:\/\/)?(.+)$/;

export function getBuildURLPathFromTags(tags: readonly string[]) {
  for (const tag of tags) {
    const [, buildId] = BUILD_ID_TAG_RE.exec(tag) || [];
    if (buildId) {
      return getBuildURLPathFromBuildId(buildId);
    }
  }

  for (const tag of tags) {
    const [, , url] = LOG_LOCATION_TAG_RE.exec(tag) || [];
    if (url) {
      return `/raw/build/${url}`;
    }
  }

  return null;
}

export function getBotUrl(swarmingHost: string, botId: string): string {
  return `https://${swarmingHost}/bot?id=${botId}`;
}
