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

import { sanitizeHTML } from './sanitize_html';

const md = MarkdownIt({html: true});

export function renderMarkdown(markdown: string) {
  return sanitizeHTML(md.render(markdown));
}

/**
 * Add an additional param to the given URL.
 */
export function urlWithSearchParam(urlStr: string, key: string, value: string, override = false): string {
  const url = new URL(urlStr);
  if (override) {
    url.searchParams.set(key, value);
  } else {
    url.searchParams.append(key, value);
  }
  return url.toString();
}
