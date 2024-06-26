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
export function toSinkResult(
  test: Test,
  testCaseResult: TestCaseResult,
  ctx: ToSinkResultContext,
): TestResult {
  const testName = [
    ...testCaseResult.ancestorTitles,
    testCaseResult.title,
  ].join(ctx.delimiter);
  const repoPath = path.join(
    ctx.directory,
    test.path.slice(test.context.config.rootDir.length + 1),
  );
  const testId = ctx.repo + ctx.delimiter + repoPath + ctx.delimiter + testName;

  return TestResult.fromPartial({
    testId: testId,
    expected: STATUS_EXPECTANCY_MAP[testCaseResult.status],
    status: STATUS_MAP[testCaseResult.status],
    summaryHtml: testCaseResult.failureMessages
      .map((msg) => {
        const msgAndStack = separateMessageFromStack(msg);
        const message = indentAllLines(msgAndStack.message);
        const stack = chalk.dim(
          formatStackTrace(
            msgAndStack.stack,
            test.context.config,
            ctx.stackTraceOpts,
            test.path,
          ),
        );
        return `<pre>${ansiConverter.toHtml(`${message}\n${stack}`)}</pre>`;
      })
      .join('\n'),
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
