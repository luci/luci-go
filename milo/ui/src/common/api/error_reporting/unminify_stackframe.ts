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
import ErrorStackParser, { StackFrame } from 'error-stack-parser';
import { SourceMapConsumer } from 'source-map';

import {
  formatFrame,
  ASTCache,
  findEnclosingFunctionName,
  getSourceMapUrl,
} from './utils';

// Cache promises to prevent race conditions and re-fetching.
const sourceMapConsumerCache = new Map<
  string,
  Promise<SourceMapConsumer | null>
>();

/**
 * Fetches, parses, and caches a source map for a given script URL.
 * @param url - The URL of the minified JavaScript file.
 * @returns A `SourceMapConsumer` instance if successful, otherwise `null`.
 */
async function getSourceMapConsumer(
  url: string | undefined,
): Promise<SourceMapConsumer | null> {
  if (!url) return null;

  // Return the cached promise if it exists.
  if (sourceMapConsumerCache.has(url)) {
    return sourceMapConsumerCache.get(url)!;
  }

  // Create a new promise, cache it immediately, and then return it.
  const promise = (async () => {
    try {
      const sourceMapUrl = getSourceMapUrl(url);
      const response = await fetch(sourceMapUrl);

      if (!response.ok) {
        throw new Error(`Failed to fetch source map: ${response.statusText}`);
      }

      const sourceMapJson = await response.json();
      return await new SourceMapConsumer(sourceMapJson);
    } catch (error) {
      // By returning null, we ensure that failed requests aren't retried constantly.
      return null;
    }
  })();
  sourceMapConsumerCache.set(url, promise);
  return promise;
}

/**
 * Processes a single stack frame and attempts to map it back to its original
 * source location. If source mapping is successful, it returns
 * a formatted string with the original file, line, and column. Otherwise, it
 * falls back to the minified stack frame information.
 *
 * @param minifiedFrame The minified stack frame.
 * @param consumerPromise A promise that resolves to a SourceMapConsumer instance.
 * @returns Original stack frame (or the minified stack frame if it failed decode it back).
 */
async function unminifyFrame(
  minifiedFrame: StackFrame,
  consumerPromise: Promise<SourceMapConsumer | null> | undefined,
  astCache: ASTCache,
): Promise<string> {
  if (!minifiedFrame.lineNumber || !minifiedFrame.columnNumber) {
    return formatFrame(minifiedFrame);
  }

  const consumer = consumerPromise ? await consumerPromise : null;
  if (!consumer) {
    return formatFrame(minifiedFrame);
  }

  const originalFrame = consumer.originalPositionFor({
    line: minifiedFrame.lineNumber,
    // The column number in the error stack is 1-based, while
    // source-map expects a 0-based.
    column: minifiedFrame.columnNumber - 1,
  });
  if (originalFrame && originalFrame.column) {
    // Change it back to 1-based.
    originalFrame.column++;

    // originalPositionFor parses the original position really good,
    // but it struggles with the enclosing function names.
    if (originalFrame.source && originalFrame.line) {
      const ast = astCache.get(originalFrame.source, consumer);
      if (ast) {
        originalFrame.name = findEnclosingFunctionName(
          ast,
          originalFrame.line,
          originalFrame.column,
        );
      }
    }
  }

  return formatFrame(minifiedFrame, originalFrame);
}

/**
 * Takes a minified Error object and attempts to translate its stack trace
 * to the original source code positions using source maps.
 * @param {Error} error - The minified Error object.
 * @returns A string representing the un-minified error message and stack trace.
 */
export async function unminifyError(error: Error): Promise<string> {
  if (!error || !error.stack) return error?.message;

  // eslint-disable-next-line import/no-named-as-default-member
  const stackFrames = ErrorStackParser.parse(error);
  // Create a map to hold the consumer promises, ensuring each is fetched only once.
  const consumerPromises = new Map<string, Promise<SourceMapConsumer | null>>();

  for (const frame of stackFrames) {
    if (frame.fileName && !consumerPromises.has(frame.fileName)) {
      consumerPromises.set(
        frame.fileName,
        getSourceMapConsumer(frame.fileName),
      );
    }
  }

  const astCache = new ASTCache();
  const unminifiedLines = await Promise.all(
    stackFrames.map((frame) =>
      unminifyFrame(
        frame,
        consumerPromises.get(frame.fileName || ''),
        astCache,
      ),
    ),
  );

  return `[Host: ${window.location.hostname}]\nError: ${error.message}\n${unminifiedLines.join('\n')}`;
}
