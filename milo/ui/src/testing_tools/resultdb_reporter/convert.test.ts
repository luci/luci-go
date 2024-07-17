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

import { TestCaseResult, TestContext } from '@jest/reporters';

import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/sink/proto/v1/test_result.pb';

import { toSinkResult } from './convert';

describe('toSinkResult', () => {
  it('compute basic properties correctly', async () => {
    const result = await toSinkResult(
      {
        context: {
          config: {
            rootDir: '/path/to/repo/path/to/directory',
            testMatch: ['**/__tests__/**/*.[jt]s?(x)', '**/*.test.[jt]s?(x)'],
          } as TestContext['config'],
        } as TestContext,
        path: '/path/to/repo/path/to/directory/path/to/test/file.test.ts',
      },
      (await import('./test_data/basic_properties.json')) as TestCaseResult,
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

  it('should clean up stack trace', async () => {
    const result = await toSinkResult(
      {
        context: {
          config: {
            rootDir: '/path/to/repo/path/to/directory',
            testMatch: ['**/__tests__/**/*.[jt]s?(x)', '**/*.test.[jt]s?(x)'],
          } as TestContext['config'],
        } as TestContext,
        path: '/path/to/repo/path/to/directory/path/to/test/file.test.ts',
      },
      (await import('./test_data/stack_trace.json')) as TestCaseResult,
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

  it('should truncate very long message', async () => {
    const result = await toSinkResult(
      {
        context: {
          config: {
            rootDir:
              '/usr/local/company/home/a_user/Workspace/infra_superproject/infra/go/src/go.chromium.org/luci/milo/ui',
            testMatch: ['**/__tests__/**/*.[jt]s?(x)', '**/*.test.[jt]s?(x)'],
          } as TestContext['config'],
        } as TestContext,
        // eslint-disable-next-line max-len
        path: '/usr/local/company/home/a_user/Workspace/infra_superproject/infra/go/src/go.chromium.org/luci/milo/ui/src/build/components/builder_table/builder_table.test.tsx',
      },
      (await import('./test_data/long_message.json')) as TestCaseResult,
      {
        repo: 'example.googlesource.com/repo',
        directory: 'milo/ui',
        delimiter: ' > ',
        stackTraceOpts: {
          noStackTrace: false,
        },
      },
    );
    expect(result).toEqual(
      TestResult.fromPartial({
        testId:
          // eslint-disable-next-line max-len
          'example.googlesource.com/repo > milo/ui/src/build/components/builder_table/builder_table.test.tsx > <BuilderTable /> > should batch calls together',
        expected: false,
        status: TestStatus.FAIL,
        summaryHtml: result.summaryHtml,
        duration: {
          seconds: '0',
          nanos: 262000000,
        },
        testMetadata: {
          name: '<BuilderTable /> > should batch calls together',
        },
        artifacts: result.artifacts,
      }),
    );
    expect(result.summaryHtml).toContain('... [message truncated]');
    const summaryBlob = new Blob([result.summaryHtml]);
    expect(summaryBlob.size).toBeLessThanOrEqual(4096);
  });
});
