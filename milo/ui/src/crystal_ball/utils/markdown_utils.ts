// Copyright 2026 The LUCI Authors.
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

import createDomPurify from 'dompurify';
import markdownIt from 'markdown-it';

const aiMd = markdownIt({
  html: false,
  linkify: false,
}).disable(['autolink', 'image', 'link', 'reference']);

const domPurify = createDomPurify(window);
domPurify.setConfig({
  ALLOWED_TAGS: [
    'blockquote',
    'br',
    'code',
    'em',
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'hr',
    'li',
    'ol',
    'p',
    'pre',
    's',
    'strong',
    'table',
    'tbody',
    'td',
    'th',
    'thead',
    'tr',
    'ul',
  ],
  ALLOWED_ATTR: ['class', 'start'],
  ALLOW_ARIA_ATTR: false,
  ALLOW_DATA_ATTR: false,
});

/**
 * Sanitizes and renders AI-generated markdown safely with raw HTML, images,
 * and links disabled to prevent prompt injection attacks (phishing links,
 * tracking beacons, raw HTML injection).
 */
export function renderAiMarkdown(markdown: string): string {
  return domPurify.sanitize(aiMd.render(markdown));
}
