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

import { StackFrame } from 'error-stack-parser';
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
