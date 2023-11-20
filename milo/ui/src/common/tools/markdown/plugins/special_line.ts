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
 * @fileoverview This file contains a MarkdownIt plugin to support applying
 * special rules to lines with the specified prefix.
 *
 * It performs the following steps.
 * 1. During inline processing of a block, add hidden tokens to mark the rule
 * to apply to each line.
 * 2. After the inline processing of the block, transform the text tokens in the
 * special lines with the provided transform function.
 */

import MarkdownIt from 'markdown-it';
import StateCore from 'markdown-it/lib/rules_core/state_core';
import StateInline from 'markdown-it/lib/rules_inline/state_inline';
import Token from 'markdown-it/lib/token';

export type TransformFn = (token: Token) => Token[];

interface Rule {
  rePrefix: RegExp;
  transformFn: TransformFn;
}

class SpecialLineRulesProcessor {
  private rules: Rule[] = [];

  addRule(rule: Rule) {
    this.rules.push(rule);
  }

  /**
   * Add special line open tokens.
   */
  tokenize = (s: StateInline) => {
    const hardBreakOnly = !s.md.options.breaks;
    const lastTokenType = s.tokens[s.tokens.length - 1]?.type;
    // Check if it's a new line.
    if (
      lastTokenType === undefined ||
      lastTokenType === 'hardbreak' ||
      (!hardBreakOnly && lastTokenType === 'softbreak')
    ) {
      const content = s.src.slice(s.pos);

      for (
        let activeRuleIndex = 0;
        activeRuleIndex < this.rules.length;
        ++activeRuleIndex
      ) {
        const rule = this.rules[activeRuleIndex];
        const prefixMatch = rule.rePrefix.exec(content);
        if (prefixMatch && prefixMatch.index === 0) {
          const prefix = prefixMatch![0];
          let token = s.push('text', '', 0);
          token.content = prefix;

          token = s.push(`special_line_${activeRuleIndex}`, '', 0);
          token.hidden = true;

          // markdown-it@13.0.2 requires all plugins to advance the cursor.
          // Add a character to src and advance the pos by 1 to avoid this
          // issue.
          if (prefix.length === 0) {
            s.src = s.src.slice(0, s.pos) + '@' + content;
            s.pos += 1;
          }

          s.pos += prefix.length;
          return true;
        }
      }
    }

    return false;
  };

  /**
   * Apply the transform rules.
   */
  postProcess = (s: StateCore) => {
    const hardBreakOnly = !s.md.options.breaks;

    for (const blockToken of s.tokens) {
      if (blockToken.type !== 'inline') {
        continue;
      }

      const newChildren: Token[] = [];
      let activeRule: Rule | null = null;
      let ruleLevel = 0;

      for (const srcToken of blockToken.children!) {
        // Apply transformFn to text tokens in the same level in the special
        // line.
        if (
          activeRule?.transformFn &&
          srcToken.type === 'text' &&
          srcToken.level === ruleLevel
        ) {
          newChildren.push(...activeRule.transformFn(srcToken));
          continue;
        }

        // When encountering a new special line, update activeRule and
        // ruleLevel.
        const match = /^special_line_(\d+)$/.exec(srcToken.type);
        if (match) {
          const ruleIndex = Number(match[1]);
          activeRule = this.rules[ruleIndex]!;
          ruleLevel = srcToken.level;

          // Omit the special line token.
          continue;
        }

        // Other tokens remain the same.
        newChildren.push(srcToken);

        // Reset activeRule and ruleLevel when starting a new line.
        if (
          srcToken.type === 'hardbreak' ||
          (!hardBreakOnly && srcToken.type === 'softbreak')
        ) {
          activeRule = null;
          ruleLevel = 0;
          continue;
        }
      }

      blockToken.children = newChildren;
    }

    return true;
  };
}

// Track the processor of each MarkdownIt instance.
const processorMap = new Map<MarkdownIt, SpecialLineRulesProcessor>();

/**
 * Apply special rules to text tokens that
 * 1. in lines satisfy the special prefix rule, and
 * 2. not enclosed by other tags.
 */
export function specialLine(
  md: MarkdownIt,
  rePrefix: RegExp,
  transformFn: TransformFn,
) {
  let processor = processorMap.get(md);

  if (!processor) {
    processor = new SpecialLineRulesProcessor();
    processorMap.set(md, processor);

    // Use a common processor for all the special rules for better performance.
    md.inline.ruler.before('text', 'special_line', processor.tokenize);
    md.core.ruler.push('special_line', processor.postProcess);
  }
  processor.addRule({ rePrefix, transformFn });
}
