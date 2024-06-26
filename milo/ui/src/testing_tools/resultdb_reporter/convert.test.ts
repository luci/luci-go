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

import { TestContext } from '@jest/reporters';

import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/sink/proto/v1/test_result.pb';

import { toSinkResult } from './convert';

describe('toSinkResult', () => {
  it('compute basic properties correctly', () => {
    const result = toSinkResult(
      {
        context: {
          config: {
            rootDir: '/path/to/repo/path/to/directory',
            testMatch: ['**/__tests__/**/*.[jt]s?(x)', '**/*.test.[jt]s?(x)'],
          } as TestContext['config'],
        } as TestContext,
        path: '/path/to/repo/path/to/directory/path/to/test/file.test.ts',
      },
      {
        ancestorTitles: ['test suite', 'child test suite'],
        duration: 1095,
        failureDetails: [],
        failureMessages: [],
        fullName: 'test suite child test suite test title',
        invocations: 1,
        location: null,
        numPassingAsserts: 1,
        retryReasons: [],
        status: 'passed',
        title: 'test title',
      },
      {
        repo: 'example.googlesource.com/repo',
        directory: 'path/to/directory',
        delimiter: ' > ',
        stackTraceOpts: {
          noStackTrace: false,
        },
      },
    );
    expect(result).toEqual(
      TestResult.fromPartial({
        testId:
          'example.googlesource.com/repo > path/to/directory/path/to/test/file.test.ts > test suite > child test suite > test title',
        expected: true,
        status: TestStatus.PASS,
        duration: {
          seconds: '1',
          nanos: 95000000,
        },
        testMetadata: {
          name: 'test suite > child test suite > test title',
        },
      }),
    );
  });

  it('should clean up stack trace', () => {
    const result = toSinkResult(
      {
        context: {
          config: {
            rootDir: '/path/to/repo/path/to/directory',
            testMatch: ['**/__tests__/**/*.[jt]s?(x)', '**/*.test.[jt]s?(x)'],
          } as TestContext['config'],
        } as TestContext,
        path: '/path/to/repo/path/to/directory/path/to/test/file.test.ts',
      },
      {
        ancestorTitles: ['test suite', 'child test suite'],
        duration: 1095,
        failureDetails: [],
        failureMessages: [
          'Error: \x1B[2mexpect(\x1B[22m\x1B[31mreceived\x1B[39m\x1B[2m).\x1B[22mnot\x1B[2m.\x1B[22mtoBeTruthy\x1B[2m()\x1B[22m\n' +
            '\n' +
            'Received: \x1B[31mtrue\x1B[39m\n' +
            '    at Object.<anonymous> (/path/to/repo/path/to/directory/path/to/test/file.test.ts:30:24)\n' +
            '    at Promise.then.completed (/path/to/repo/path/to/directory/node_modules/jest-circus/build/utils.js:298:28)\n' +
            '    at new Promise (<anonymous>)\n' +
            '    at callAsyncCircusFn (/path/to/repo/path/to/directory/node_modules/jest-circus/build/utils.js:231:10)\n' +
            '    at _callCircusTest (/path/to/repo/path/to/directory/node_modules/jest-circus/build/run.js:316:40)\n' +
            '    at async _runTest (/path/to/repo/path/to/directory/node_modules/jest-circus/build/run.js:252:3)\n' +
            '    at async _runTestsForDescribeBlock (/path/to/repo/path/to/directory/node_modules/jest-circus/build/run.js:126:9)\n' +
            '    at async _runTestsForDescribeBlock (/path/to/repo/path/to/directory/node_modules/jest-circus/build/run.js:121:9)\n' +
            '    at async _runTestsForDescribeBlock (/path/to/repo/path/to/directory/node_modules/jest-circus/build/run.js:121:9)\n' +
            '    at async run (/path/to/repo/path/to/directory/node_modules/jest-circus/build/run.js:71:3)\n' +
            // eslint-disable-next-line max-len
            '    at async runAndTransformResultsToJestFormat (/path/to/repo/path/to/directory/node_modules/jest-circus/build/legacy-code-todo-rewrite/jestAdapterInit.js:122:21)\n' +
            // eslint-disable-next-line max-len
            '    at async jestAdapter (/path/to/repo/path/to/directory/node_modules/jest-circus/build/legacy-code-todo-rewrite/jestAdapter.js:74:19)\n' +
            '    at async runTestInternal (/path/to/repo/path/to/directory/node_modules/jest-runner/build/runTest.js:281:16)\n' +
            '    at async runTest (/path/to/repo/path/to/directory/node_modules/jest-runner/build/runTest.js:341:7)',
        ],
        fullName: 'test suite child test suite test title',
        invocations: 1,
        location: null,
        numPassingAsserts: 1,
        retryReasons: [],
        status: 'failed',
        title: 'test title',
      },
      {
        repo: 'example.googlesource.com/repo',
        directory: 'path/to/directory',
        delimiter: ' > ',
        stackTraceOpts: {
          noStackTrace: false,
        },
      },
    );
    expect(result).toEqual(
      TestResult.fromPartial({
        testId:
          'example.googlesource.com/repo > path/to/directory/path/to/test/file.test.ts > test suite > child test suite > test title',
        expected: false,
        status: TestStatus.FAIL,
        summaryHtml: result.summaryHtml,
        duration: {
          seconds: '1',
          nanos: 95000000,
        },
        testMetadata: {
          name: 'test suite > child test suite > test title',
        },
      }),
    );
    expect(result.summaryHtml).not.toContain('/path/to/repo');
    expect(result.summaryHtml).not.toContain('/node_modules/');
  });
});
