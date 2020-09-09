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

/**
 * @fileoverview This file contains a MarkdownIt plugin to convert bug
 * identifiers (e.g. 'proj/123', '234') in bug lines (e.g. 'Bug: proj/123, 234')
 * to crbug links.
 *
 * It performs the following steps.
 * 1. During inline processing of a block, add hidden tokens to mark the start
 * and end of bug lines.
 * 2. After the inline processing of the block, convert bug identifiers in text
 * tokens in bug lines to crbug links.
 *
 * For instance
 * ```
 * Bug: 123, 234
 * ```
 * will be converted to
 * ```html
 * <p>Bug:
 *   <a href="https://crbug.com/123">123</a>
 *   <a href="https://crbug.com/234">234</a>
 * </p>
 * ```
 */

import MarkdownIt from 'markdown-it';
import StateCore from 'markdown-it/lib/rules_core/state_core';
import StateInline from 'markdown-it/lib/rules_inline/state_inline';
import Token from 'markdown-it/lib/token';

/**
 * Add bug_line_open and bug_line_close tokens.
 *
 * Note that bug_line_open tokens do not always have corresponding
 * bug_line_close tokens. In that case, the rest of the scope is considered in
 * the bug line.
 *
 * In the same scope and on the same level, there's always a bug_link_close
 * token between any two bug_line_open tokens.
 */
function tokenize(s: StateInline): boolean {
  const lineDelimiter = s.md.options.breaks ? '\n' : '  \n';

  // Check if it's a new line.
  if (s.pos === 0 || s.src.slice(s.pos - lineDelimiter.length, s.pos) === lineDelimiter) {
    const content = s.src.slice(s.pos);
    const reBugLine = /^(>| )*(bugs?)[:=]/i;
    const bugLineMatch = reBugLine.exec(content);
    if (bugLineMatch) {
      // insert prefix
      const prefix = bugLineMatch[0];
      let token = s.push('text', '', 0);
      token.content = prefix;

      token = s.push('bug_line_open', '', 0);
      token.hidden = true;

      s.pos += prefix.length;

      return true;
    }
  }

  // Check if it's the end of a bug line.
  if (s.tokens.length > 0 && s.src.slice(s.pos, s.pos + lineDelimiter.length) === lineDelimiter && isInBugLine(s.tokens, s.tokens.length - 1)) {
    const token = s.push('bug_line_close', '', 0);
    token.hidden = true;
  }

  return false;
}

/**
 * Replace text tokens in bug lines with bug links.
 */
function postProcess(s: StateCore): boolean {
  for (const blockToken of s.tokens) {
    if (blockToken.type !== 'inline') {
      continue;
    }
    const tokens = blockToken.children!;

    // Process in reverse order so newly inserted nodes do not get processed.
    for (let i = tokens.length - 1; i >= 0; --i) {
      const srcToken = tokens[i];
      if (srcToken.type !== 'text' || !isInBugLine(tokens, i)) {
        continue;
      }
      const reBug = /\b(\w+:)?#?\d+\b/g;
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
        token.attrs = [['href', `https://crbug.com/${bugStr.replace('#', '').replace(':', '/')}`]];
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

      blockToken.children!.splice(i, 1, ...newTokens);
    }
  }
  return true;
}

/**
 * Check whether the token at the given index is in the bug line.
 */
function isInBugLine(tokens: Token[], index: number) {
  const level = tokens[index].level;
  for (let i = index; i >= 0; --i) {
    const target = tokens[i];
    if (target.level > level) {
      // Token is in a inner scope, ignore.
      continue;
    } else if (target.level < level) {
      // The current scope is already closed, no point checking further.
      return false;
    } else if (target.type === 'bug_line_close') {
      return false;
    }

    if (target.type === 'bug_line_open') {
      return true;
    }
  }
  return false;
}

/**
 * Support converting bug identifiers (e.g. 'proj/123', '234') in bug lines
 * (e.g. 'Bugs: proj/123, 234', 'BUG=345') to crbug links.
 */
export function bugLine(md: MarkdownIt) {
  md.inline.ruler.before('text', 'bug_line', tokenize);
  md.core.ruler.push('bug_line', postProcess);
}
