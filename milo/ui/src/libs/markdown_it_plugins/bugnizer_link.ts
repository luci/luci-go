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

const RE_BUGNIZER = /\bb[:/]\d+\b/i;
const RE_BUGNIZER_TAIL = /^[:/]\d+\b/i;

/**
 * Support converting bugnizer link texts (e.g. 'b/123', 'b:234',
 * 'B:123') to links.
 *
 * You MUST enable linkify for this to work.
 */
export function bugnizerLink(md: MarkdownIt) {
  md.linkify.add('b', {
    validate: RE_BUGNIZER_TAIL,
    normalize: (match) => {
      const reMatch = RE_BUGNIZER.exec(match.raw)!;
      const path = reMatch[0].replace(':', '/');
      match.url = 'http://' + path;
    },
  });
}
