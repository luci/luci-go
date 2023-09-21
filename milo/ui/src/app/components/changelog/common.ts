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

import markdownIt from 'markdown-it';

import { defaultTarget } from '@/common/tools/markdown/plugins/default_target';

export interface Changelog {
  /**
   * The version of the latest changelog section.
   * -1 if there's no latest changelog.
   */
  readonly latestVersion: number;
  readonly latest: string;
  readonly past: string;
}

const RELEASE_SPLITTER_RE =
  /(?<=^|\r\n|\r|\n)(<!-- __RELEASE__: (\d)+ -->(?:$|\r\n|\r|\n))/;

export function parseChangelog(changelog: string): Changelog {
  const [unreleased, latestVersionTag, latestVersion, latestContent] =
    changelog.split(RELEASE_SPLITTER_RE, 4);

  if (!latestVersionTag) {
    return {
      latestVersion: -1,
      latest: '',
      past: '',
    };
  }

  const latest = latestVersionTag + latestContent;
  return {
    latestVersion: Number(latestVersion),
    latest,
    past: changelog.slice(unreleased.length + latest.length),
  };
}

const md = markdownIt({ html: true, linkify: true }).use(
  defaultTarget,
  '_blank',
);

export function renderChangelog(changelog: string) {
  return md.render(changelog);
}
