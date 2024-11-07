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
import Token from 'markdown-it/lib/token.mjs';

import { specialLine } from './special_line';

/**
 * Replace text tokens with bug links.
 */
function processToken(srcToken: Token): Token[] {
  const reBug = /\b([-_a-zA-Z0-9]+:)?#?\d+\b/g;
  let pos = 0;
  let match = reBug.exec(srcToken.content);
  const newTokens: Token[] = [];
  let token: Token;
  while (match) {
    const bugStr = match[0];

    // Insert the text between the last processed character and the first
    // character of the new match as a text node.
    if (match.index !== pos) {
      token = new Token('text', '', 0);
      token.content = srcToken.content.slice(pos, match.index);
      newTokens.push(token);
      pos = match.index;
    }

    // Insert the bug link nodes.
    token = new Token('link_open', 'a', 1);
    token.attrs = [
      [
        'href',
        `https://crbug.com/${bugStr.replace('#', '').replace(':', '/')}`,
      ],
    ];
    token.level = srcToken.level;
    newTokens.push(token);

    token = new Token('text', '', 0);
    token.content = bugStr;
    token.level = srcToken.level + 1;
    pos += bugStr.length;
    newTokens.push(token);

    token = new Token('link_close', 'a', -1);
    token.level = srcToken.level;
    newTokens.push(token);

    match = reBug.exec(srcToken.content);
  }

  // If there are remaining unprocessed characters, add them as a text node.
  if (pos !== srcToken.content.length) {
    token = new Token('text', '', 0);
    token.content = srcToken.content.slice(pos);
    newTokens.push(token);
  }

  return newTokens;
}

/**
 * Support converting bug identifiers (e.g. 'proj/123', '234') in bug lines
 * (e.g. 'Bugs: proj/123, 234', 'BUG=345') to crbug links.
 *
 * For instance
 * ```
 * Bug: 123, proj/234
 * ```
 * will be converted to
 * ```html
 * <p>Bug:
 *   <a href="https://crbug.com/123">123</a>
 *   <a href="https://crbug.com/proj/234">proj/234</a>
 * </p>
 * ```
 */
export function bugLine(md: MarkdownIt) {
  md.use(specialLine, /^(>| )*(bugs?)[:=]/i, processToken);
}
