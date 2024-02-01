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

import { logging } from '@/common/tools/logging';
import { defaultTarget } from '@/common/tools/markdown/plugins/default_target';

export interface ReleaseNotes {
  /**
   * The version of the latest release note section.
   * -1 if there's no latest release note.
   */
  readonly latestVersion: number;
  readonly latest: string;
  readonly past: string;
}

const RELEASE_SPLITTER_RE =
  /(?<=^|\r\n|\r|\n)(<!-- __RELEASE__: (\d)+ -->(?:$|\r\n|\r|\n))/;

export function parseReleaseNotes(releaseNote: string): ReleaseNotes {
  const [unreleased, latestVersionTag, latestVersion, latestContent] =
    releaseNote.split(RELEASE_SPLITTER_RE, 4);

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
    past: releaseNote.slice(unreleased.length + latest.length),
  };
}

const md = markdownIt({ html: true, linkify: true }).use(
  defaultTarget,
  '_blank',
);

export function renderReleaseNotes(releaseNotes: string) {
  return md.render(releaseNotes);
}

const LAST_READ_RELEASE_VERSION_CACHE_KEY = 'last-read-release-version';

/**
 * Get the last read version from local storage. If it's not set or the value
 * is invalid, return -1.
 */
export function getLastReadVersion() {
  try {
    const ver = JSON.parse(
      localStorage.getItem(LAST_READ_RELEASE_VERSION_CACHE_KEY) || '-1',
    );
    return typeof ver === 'number' ? ver : -1;
  } catch (e) {
    logging.warn(e);
    return -1;
  }
}

/**
 * Update the last read version. Noop if newVer is less than or equals to the
 * stored last read version.
 */
// Do not use `useLocalStorage` from react-use. Otherwise we may risk
// overwriting the version set by older versions of the UI, causing the popups
// to constantly show up when user switch between pages of different versions.
export function bumpLastReadVersion(newVer: number) {
  // In case another tab has recorded a newer version, get a fresh copy from the
  // local storage.
  const prevVer = getLastReadVersion();
  if (newVer < prevVer) {
    return;
  }

  localStorage.setItem(
    LAST_READ_RELEASE_VERSION_CACHE_KEY,
    JSON.stringify(newVer),
  );
}
