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

import * as ast from '../ast/ast';

/**
 * Serializes an AIP-160 AST Node back into a valid AIP-160 filter query string.
 */
export function printFilter(node: ast.Node | null): string {
  if (!node) return '';

  switch (node.kind) {
    case 'Filter':
      return node.expression ? printFilter(node.expression) : '';

    case 'Expression':
      return node.sequences.map(printFilter).filter(Boolean).join(' AND ');

    case 'Sequence':
      return node.factors.map(printFilter).filter(Boolean).join(' ');

    case 'Factor':
      return node.terms.map(printFilter).filter(Boolean).join(' OR ');

    case 'Term': {
      const prefix = node.negated ? '-' : '';
      if (node.simple.kind === 'Expression') {
        return `${prefix}(${printFilter(node.simple)})`;
      }
      return `${prefix}${printFilter(node.simple)}`;
    }

    case 'Restriction': {
      const compStr = printFilter(node.comparable);
      if (!node.comparator || !node.arg) {
        return compStr;
      }
      const argStr =
        node.arg.kind === 'Expression'
          ? `(${printFilter(node.arg)})`
          : printFilter(node.arg);

      if (node.comparator === ':') {
        return `${compStr}:${argStr}`;
      }
      return `${compStr} ${node.comparator} ${argStr}`;
    }

    case 'Comparable':
      return printFilter(node.member);

    case 'Member': {
      const rootStr = printFilter(node.value);
      if (node.fields.length === 0) {
        return rootStr;
      }
      return `${rootStr}.${node.fields.map(printFilter).join('.')}`;
    }

    case 'Value':
      if (node.quoted) {
        return JSON.stringify(node.value);
      }
      return node.value;
  }
}
