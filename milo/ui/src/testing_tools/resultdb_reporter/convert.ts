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
import chalk from 'chalk';
import {
  StackTraceOptions,
  formatStackTrace,
  indentAllLines,
  separateMessageFromStack,
} from 'jest-message-util';

import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import {
  TestIdentifier,
  TestResult,
} from '@/proto/go.chromium.org/luci/resultdb/sink/proto/v1/test_result.pb';

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

export interface ToSinkResultContext {
  readonly repo: string;
  readonly directory: string;
  readonly delimiter: string;
  readonly bugComponentId?: string;

  readonly stackTraceOpts: StackTraceOptions;
}

function prepareTestIdStructured(
  testPath: string,
  testCaseParts: string[],
): TestIdentifier {
  const lastIndex = testPath.lastIndexOf('/');
  let coarseName: string;
  let fineName: string;
  if (lastIndex !== -1) {
    coarseName = testPath.slice(0, lastIndex) + '/';
    fineName = testPath.slice(lastIndex + 1);
  } else {
    coarseName = '/';
    fineName = testPath;
  }

  return TestIdentifier.fromPartial({
    coarseName,
    fineName,
    caseNameComponents: testCaseParts,
  });
}

/**
 * Converts a Jest test result to a Result Sink test result.
 */
export async function toSinkResult(
  test: Test,
  testCaseResult: TestCaseResult,
  ctx: ToSinkResultContext,
): Promise<TestResult> {
  const testCaseParts = [
    ...testCaseResult.ancestorTitles,
    testCaseResult.title,
  ];
  const testName = testCaseParts.join(ctx.delimiter);

  // The test path relative to the directory we are executing from.
  const testPath = test.path.slice(test.context.config.rootDir.length + 1);

  // The test path from the repository root.
  const repoPath = path.join(ctx.directory, testPath);
  const testId = ctx.repo + ctx.delimiter + repoPath + ctx.delimiter + testName;
  const failureMessages = testCaseResult.failureMessages
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

      return `${message}\n\n${stack}`;
    })
    .join('\n');

  return TestResult.fromPartial({
    testId: testId,
    testIdStructured: prepareTestIdStructured(testPath, testCaseParts),
    expected: STATUS_EXPECTANCY_MAP[testCaseResult.status],
    status: STATUS_MAP[testCaseResult.status],
    // Minify the HTML. This helps but not by a lot. If we want better
    // minification, we need to make the ANSI to HTML converter generate simpler
    // HTML.
    summaryHtml: failureMessages
      ? '<text-artifact artifact-id="failure-messages" experimental-ansi-support></text-artifact>'
      : '',
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
    artifacts: {
      ...(testCaseResult.failureDetails.length > 0 && {
        ['failure-details']: {
          contentType: 'application/json',
          contents: Buffer.from(JSON.stringify(testCaseResult.failureDetails)),
        },
      }),
      ...(failureMessages !== '' && {
        'failure-messages': {
          contentType: 'text/x-ansi',
          contents: Buffer.from(failureMessages),
        },
      }),
    },

    // TODO: generate failure reason.
    failureReason: undefined,
  });
}
