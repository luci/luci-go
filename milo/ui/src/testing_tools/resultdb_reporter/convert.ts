// Copyright 2024 The LUCI Authors.
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

import * as path from 'node:path';

import { Test, TestCaseResult } from '@jest/reporters';
import ANSIConverter from 'ansi-to-html';
import chalk from 'chalk';
import { minify, Options } from 'html-minifier-terser';
import {
  StackTraceOptions,
  formatStackTrace,
  indentAllLines,
  separateMessageFromStack,
} from 'jest-message-util';

import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/sink/proto/v1/test_result.pb';

const STATUS_EXPECTANCY_MAP = Object.freeze({
  passed: true,
  failed: false,
  skipped: true,
  pending: true,
  todo: true,
  disabled: true,
  focused: true,
});

const STATUS_MAP = Object.freeze({
  passed: TestStatus.PASS,
  failed: TestStatus.FAIL,
  skipped: TestStatus.SKIP,
  pending: TestStatus.SKIP,
  todo: TestStatus.SKIP,
  disabled: TestStatus.SKIP,
  focused: TestStatus.SKIP,
});

const ansiConverter = new ANSIConverter({
  bg: '#FFF',
  fg: '#000',
  newline: true,
});

/**
 * The maximum message length.
 *
 * Pick a number such that the generated HTML after the stack trace is added
 * will not exceed the `TestResult.summary_html`'s size limit (4096 bytes as of
 * 2024-07-17).
 */
const MAX_MESSAGE_LENGTH = 1024;

const MINIFY_OPTIONS = Object.freeze<Options>({
  collapseBooleanAttributes: true,
  collapseWhitespace: true,
  conservativeCollapse: true,
  preserveLineBreaks: true,
  minifyCSS: true,
  minifyURLs: true,
  removeComments: true,
  removeEmptyAttributes: true,
  removeEmptyElements: true,
  removeOptionalTags: true,
  removeRedundantAttributes: true,
  removeStyleLinkTypeAttributes: true,
  useShortDoctype: true,
});

export interface ToSinkResultContext {
  readonly repo: string;
  readonly directory: string;
  readonly delimiter: string;
  readonly bugComponentId?: string;

  readonly stackTraceOpts: StackTraceOptions;
}

/**
 * Converts a Jest test result to a Result Sink test result.
 */
export async function toSinkResult(
  test: Test,
  testCaseResult: TestCaseResult,
  ctx: ToSinkResultContext,
): Promise<TestResult> {
  const testName = [
    ...testCaseResult.ancestorTitles,
    testCaseResult.title,
  ].join(ctx.delimiter);
  const repoPath = path.join(
    ctx.directory,
    test.path.slice(test.context.config.rootDir.length + 1),
  );
  const testId = ctx.repo + ctx.delimiter + repoPath + ctx.delimiter + testName;
  const summaryHtml = testCaseResult.failureMessages
    .map((msg) => {
      const msgAndStack = separateMessageFromStack(msg);
      let message = indentAllLines(msgAndStack.message);

      // Truncate the message if it's too long.
      if (message.length > MAX_MESSAGE_LENGTH) {
        message = message.slice(0, MAX_MESSAGE_LENGTH);
        const lastEscapeIndex = message.lastIndexOf('\u001B');
        // Truncate to the nearest escape sequence if possible.
        // This is to prevent partial inclusion of ANSI codes.
        if (lastEscapeIndex > 0) {
          message = message.slice(0, lastEscapeIndex);
        }
        message += '\n    \u001B[39;103m... [message truncated]';
      }

      const stack = chalk.dim(
        formatStackTrace(
          msgAndStack.stack,
          test.context.config,
          ctx.stackTraceOpts,
          test.path,
        ),
      );

      // Convert each line individually to prevent `ansi-to-html`'s excessive
      // `<span>` wrapping.
      const lines = [...message.split('\n'), ...stack.split('\n')]
        .map((line) => ansiConverter.toHtml(line))
        .join('\n');
      return `<pre>${lines}</pre>`;
    })
    .join('\n');

  return TestResult.fromPartial({
    testId: testId,
    expected: STATUS_EXPECTANCY_MAP[testCaseResult.status],
    status: STATUS_MAP[testCaseResult.status],
    // Minify the HTML. This helps but not by a lot. If we want better
    // minification, we need to make the ANSI to HTML converter generate simpler
    // HTML.
    summaryHtml: await minify(summaryHtml, MINIFY_OPTIONS),
    duration:
      typeof testCaseResult.duration === 'number'
        ? {
            seconds: String(Math.floor(testCaseResult.duration / 1000)),
            nanos: (testCaseResult.duration % 1000) * 1_000_000,
          }
        : undefined,
    testMetadata: {
      name: testName,
      location: testCaseResult.location
        ? {
            repo: ctx.repo,
            fileName: '//' + repoPath,
            line: testCaseResult.location.line,
          }
        : undefined,
      bugComponent: ctx.bugComponentId
        ? {
            issueTracker: { componentId: ctx.bugComponentId },
          }
        : undefined,
    },
    artifacts:
      testCaseResult.failureDetails.length > 0
        ? {
            failureDetails: {
              contents: Buffer.from(
                JSON.stringify(testCaseResult.failureDetails),
              ),
              contentType: 'application/json',
            },
          }
        : undefined,
    // TODO: generate failure reason.
    failureReason: undefined,
  });
}
