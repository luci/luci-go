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

import { defaultTarget } from './markdown_it_plugins/default_target';
import { sanitizeHTML } from './sanitize_html';

const md = MarkdownIt({html: true})
  .use(defaultTarget, '_blank');

export function renderMarkdown(markdown: string) {
  return sanitizeHTML(md.render(markdown));
}

/**
 * Extend URL with methods that can be chained.
 */
export class ChainableURL extends URL {
  withSearchParam(key: string, value: string, override = false) {
    if (override) {
      this.searchParams.set(key, value);
    } else {
      this.searchParams.append(key, value);
    }
    return this;
  }
}

/**
 * Compares the size of two big unsigned integers represented by strings.
 */
export function compareBigInt(num1: string, num2: string): number {
  const maxLen = Math.max(num1.length, num2.length);
  num1 = num1.padStart(maxLen, '0');
  num2 = num2.padStart(maxLen, '0');
  return num1.localeCompare(num2);
}
