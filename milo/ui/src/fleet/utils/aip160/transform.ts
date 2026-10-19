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

import * as ast from './ast/ast';
import { parseFilter } from './parser/parser';
import { printFilter } from './printer/printer';

/**
 * Transforms an AIP-160 filter string by remapping top-level dimension field names
 * (e.g. `model` -> `labels.model`, `pool` -> `labels.pool`, `device_id` -> `id`)
 * using the abstract syntax tree.
 *
 * It leaves values, operators, and already qualified fields (like `labels.model`) intact.
 * If the input string is not a valid AIP-160 filter, it falls back gracefully to the original string.
 */
export function remapFilterDimensions(
  filter: string | undefined | null,
  mapping: Record<string, string>,
): string {
  if (!filter || !filter.trim()) return '';

  const parsed = parseFilter(filter);
  if (parsed.isError || !parsed.ast.expression) {
    return filter;
  }

  const transformNode = <T extends ast.Node>(node: T): T => {
    switch (node.kind) {
      case 'Filter':
        return {
          ...node,
          expression: node.expression ? transformNode(node.expression) : null,
        };
      case 'Expression':
        return {
          ...node,
          sequences: node.sequences.map(transformNode),
        };
      case 'Sequence':
        return {
          ...node,
          factors: node.factors.map(transformNode),
        };
      case 'Factor':
        return {
          ...node,
          terms: node.terms.map(transformNode),
        };
      case 'Term':
        return {
          ...node,
          simple: transformNode(node.simple),
        };
      case 'Restriction': {
        const transformedComp = transformComparable(node.comparable);
        const transformedArg = node.arg ? transformArg(node.arg) : null;
        return {
          ...node,
          comparable: transformedComp,
          arg: transformedArg,
        };
      }
      default:
        return node;
    }
  };

  const transformArg = (arg: ast.Arg): ast.Arg => {
    if (arg.kind === 'Expression') {
      return transformNode(arg);
    }
    return arg;
  };

  const transformComparable = (comp: ast.Comparable): ast.Comparable => {
    const member = comp.member;

    // Only remap unqualified top-level fields (e.g. `model`, `pool`, `device_id`),
    // leaving already qualified labels (e.g. `labels.model`) untouched.
    if (member.fields.length === 0 && !member.value.quoted) {
      const rootKey = member.value.value;
      const target = mapping[rootKey];
      if (target) {
        const parts = target.split('.');
        const newRoot: ast.Value = {
          kind: 'Value',
          value: parts[0],
          quoted: false,
        };
        const newFields: ast.Value[] = parts.slice(1).map((f) => ({
          kind: 'Value',
          value: f.replace(/^"|"$/g, ''),
          quoted: f.startsWith('"') && f.endsWith('"'),
        }));

        return {
          ...comp,
          member: {
            ...member,
            value: newRoot,
            fields: newFields,
          },
        };
      }
    }
    return comp;
  };

  const transformedAst = transformNode(parsed.ast);
  return printFilter(transformedAst);
}
