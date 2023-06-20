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

const APP_VERSION_RE = /^(\d+)-([0-9a-f]+)(-.+)?$/;

export interface ParsedAppVersion {
  readonly num: number;
  readonly hash: string;
}

/**
 * Parse the version string with the format of `APP_VERSION_RE`.
 *
 * If the version string is invalid, return null.
 */
export function parseAppVersion(version: string): ParsedAppVersion | null {
  const match = version.match(APP_VERSION_RE);
  if (!match) {
    return null;
  }

  const [, num, hash] = match;
  return {
    num: Number(num),
    hash,
  };
}
