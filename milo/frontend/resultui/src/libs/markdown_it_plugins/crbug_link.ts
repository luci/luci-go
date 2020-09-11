// Copyright 2020 The LUCI Authors.
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

import MarkdownIt from 'markdown-it';

const RE_CRBUG = /\bcrbug(\.com)?(([:/]\w+)?[:/]\d+)\b/i;
const RE_CRBUG_TAIL = /^(\.com)?([:/]\w+)?[:/]\d+\b/i;

/**
 * Support converting crbug link texts (e.g. 'crbug/123', 'crbug.com/234',
 * 'crbug.com:proj:123') to links.
 *
 * You MUST enable linkify for this to work.
 */
export function crbugLink(md: MarkdownIt) {
  md.linkify.add('crbug', {
    validate: RE_CRBUG_TAIL,
    normalize: (match) => {
      const reMatch = RE_CRBUG.exec(match.raw)!;
      const path = reMatch[2].replace(':', '/');
      match.url = 'https://crbug.com' + path;
    },
  });
}
