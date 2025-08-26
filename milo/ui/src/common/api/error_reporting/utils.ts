// Copyright 2025 The LUCI Authors.
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

import { parse as babelParse } from '@babel/parser';
import traverse from '@babel/traverse';
import type { File as BabelAst } from '@babel/types';
import { StackFrame } from 'error-stack-parser';
import type { SourceMapConsumer } from 'source-map';
import { NullableMappedPosition } from 'source-map';
export const ANONYMOUS_CALLABLE = '<anonymous>';

/**
 * Derives the source map url from the minified file url
 */
export function getSourceMapUrl(minifiedFileUrl: string): string {
  const cleanUrl = new URL(minifiedFileUrl);
  cleanUrl.search = '';
  cleanUrl.hash = '';
  return `${cleanUrl.toString()}.map`;
}

export function trimLeadingParents(path: string) {
  // Example: '../../src/components/button.js' becomes 'src/components/button.js'
  return path.replace(/^(\.\.\/)+/, '');
}

/**
 * Formats a stack frame for logging.
 *
 * @param frame The raw stack frame.
 * @param originalPos The original position found from a source map, if available.
 * @returns A formatted stack trace line.
 */
export function formatFrame(
  minifiedFrame: StackFrame,
  originalFrame?: NullableMappedPosition,
): string {
  if (
    !originalFrame ||
    !originalFrame.source ||
    !originalFrame.line ||
    !originalFrame.column
  ) {
    // Return the minified frame if we didn't succeed with the resolution.
    return `    at ${minifiedFrame.toString()}`;
  }

  const location = `${trimLeadingParents(originalFrame.source)}:${originalFrame.line}:${originalFrame.column}`;
  const functionName =
    originalFrame.name || minifiedFrame.functionName || ANONYMOUS_CALLABLE;
  return `    at ${functionName} (${location})`;
}

/**
 * Finds the name of the most specific enclosing function/method for a given
 * location in a pre-parsed Babel AST.
 */
export function findEnclosingFunctionName(
  ast: BabelAst,
  line: number,
  column: number,
): string | null {
  let functionName: string | null = null;
  let smallestNodeSize = Infinity;

  traverse(ast, {
    enter(path) {
      if (!path.node.loc) return;

      const { start, end } = path.node.loc;
      const nodeSize =
        (end.line - start.line) * 1000 + (end.column - start.column);
      const isWithin =
        start.line <= line &&
        end.line >= line &&
        (start.line === line ? start.column <= column : true) &&
        (end.line === line ? end.column >= column : true);

      if (!isWithin || nodeSize >= smallestNodeSize) {
        // If the node is not a better scope, we can skip traversing its children.
        path.skip();
        return;
      }

      // Check for various function types and extract the name
      let newName: string | null = null;
      if (path.isFunctionDeclaration() && path.node.id) {
        // Handles: function MyFunction() {}
        newName = path.node.id.name;
      } else if (path.isFunctionExpression() && path.node.id) {
        // Handles: const x = function MyFunction() {}
        newName = path.node.id.name;
      } else if (path.isClassMethod() || path.isObjectMethod()) {
        // Handles class and object methods: class C { myMethod() {} }
        if (path.node.key.type === 'Identifier') {
          newName = path.node.key.name;
        }
      } else if (
        path.isArrowFunctionExpression() ||
        path.isFunctionExpression()
      ) {
        const parent = path.parent;
        if (
          parent.type === 'VariableDeclarator' &&
          parent.id.type === 'Identifier'
        ) {
          // Handles: const MyFunction = () => {}
          newName = parent.id.name;
        } else if (parent.type === 'ObjectProperty') {
          if (parent.key.type === 'Identifier') {
            // Handles: { myMethod: () => {} }
            newName = parent.key.name;
          } else if (parent.key.type === 'StringLiteral') {
            // Handles: { "my-method": () => {} }
            newName = parent.key.value;
          }
        } else if (parent.type === 'ClassProperty') {
          if (parent.key.type === 'Identifier') {
            // Handles: myMethod = () => {} inside a class
            newName = parent.key.name;
          } else if (parent.key.type === 'StringLiteral') {
            // Handles: "my-method": () => {} inside a class
            newName = parent.key.value;
          }
        } else if (
          parent.type === 'JSXExpressionContainer' &&
          path.parentPath?.parent.type === 'JSXAttribute'
        ) {
          // Handles inline functions in JSX props like onClick={() => ...}
          const attributeNode = path.parentPath.parent;
          if (attributeNode.name.type === 'JSXIdentifier') {
            newName = attributeNode.name.name;
          }
        }
      }

      if (newName) {
        functionName = newName;
        smallestNodeSize = nodeSize;
      }
    },
  });
  return functionName;
}

export class ASTCache {
  private cache = new Map<string, BabelAst | null>();

  get(source: string, sourceMapConsumer: SourceMapConsumer): BabelAst | null {
    if (this.cache.has(source)) {
      return this.cache.get(source)!;
    }

    const sourceCode = sourceMapConsumer.sourceContentFor(source, true);
    if (!sourceCode) {
      this.cache.set(source, null);
      return null;
    }

    try {
      const ast = babelParse(sourceCode, {
        sourceType: 'module',
        plugins: ['typescript', 'jsx'],
      });

      this.cache.set(source, ast);
      return ast;
    } catch (err) {
      this.cache.set(source, null);
      return null;
    }
  }
}

export function getFleetConsoleProject() {
  let fleetConsoleProjectApiKey = '';
  let fleetConsoleProjectId = '';

  if (
    ['luci-milo.appspot.com', 'ci.chromium.org'].includes(
      window.location.hostname,
    )
  ) {
    fleetConsoleProjectApiKey = 'AIzaSyD8sLpokusZlnCyEDZ_b3dZmcSueo5g_-M';
    fleetConsoleProjectId = 'fleet-console-prod';
  } else if (window.location.hostname.endsWith('luci-milo-dev.appspot.com')) {
    fleetConsoleProjectApiKey = 'AIzaSyDxnTm4WP4mpJ5t1_S-q68vbGc8YEDPVrA';
    fleetConsoleProjectId = 'fleet-console-dev';
  }
  return { fleetConsoleProjectApiKey, fleetConsoleProjectId };
}
