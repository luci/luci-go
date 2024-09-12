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

import { fireEvent, render, screen } from '@testing-library/react';
import { act } from 'react';

import { OutputTestVariantBranch } from '@/analysis/types';
import {
  CommitTable,
  CommitTableBody,
  CommitTableHead,
} from '@/gitiles/components/commit_table';
import { OutputCommit } from '@/gitiles/types';
import {
  QuerySourceVerdictsResponse_SourceVerdict,
  QuerySourceVerdictsResponse_VerdictStatus,
  TestVariantBranch,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { Commit } from '@/proto/go.chromium.org/luci/common/proto/git/commit.pb';
import {
  BatchGetTestVariantsResponse,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BlamelistContextProvider } from './context';
import { BlamelistTableRow, BlamelistTableHeaderContent } from './row';

const testVariantBranch = TestVariantBranch.fromPartial({
  project: 'proj',
  testId: 'test-id',
  variantHash: 'v-hash',
  refHash: 'ref-hash',
}) as OutputTestVariantBranch;

const sourceVerdict = QuerySourceVerdictsResponse_SourceVerdict.fromPartial({
  status: QuerySourceVerdictsResponse_VerdictStatus.FLAKY,
  verdicts: [
    {
      invocationId: 'inv1',
      status: QuerySourceVerdictsResponse_VerdictStatus.EXPECTED,
    },
    {
      invocationId: 'inv2',
      status: QuerySourceVerdictsResponse_VerdictStatus.UNEXPECTED,
    },
  ],
});

const commit = Commit.fromPartial({
  id: 'abcd',
  committer: {
    email: 'committer@email.com',
    name: 'committer',
    time: '2020-02-02',
  },
  author: {
    email: 'author@email.com',
    name: 'author',
    time: '2020-02-02',
  },
  message: 'a commit',
  treeDiff: [{ newPath: 'file-name' }],
}) as OutputCommit;

function TestComponent() {
  return (
    <BlamelistContextProvider testVariantBranch={testVariantBranch}>
      <CommitTable repoUrl="https://repo.url">
        <CommitTableHead>
          <BlamelistTableHeaderContent />
        </CommitTableHead>
        <CommitTableBody>
          <BlamelistTableRow
            testVariantBranch={testVariantBranch}
            commit={commit}
            position="1234"
            sourceVerdict={sourceVerdict}
            isSvLoading={false}
          />
        </CommitTableBody>
      </CommitTable>
    </BlamelistContextProvider>
  );
}

describe('<BlamelistTableContentRow />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest
      .spyOn(ResultDBClientImpl.prototype, 'BatchGetTestVariants')
      .mockImplementation(async (req) =>
        BatchGetTestVariantsResponse.fromPartial({
          testVariants: [
            {
              testId: 'test-id',
              status: TestVariantStatus.UNEXPECTED,
              results: [
                {
                  result: {
                    name: `invocations/build-5678/tests/test-id/results/1`,
                    summaryHtml: `Result #1 from ${req.invocation}`,
                    expected: false,
                    status: TestStatus.FAIL,
                  },
                },
              ],
            },
            {
              testId: 'test-id',
              status: TestVariantStatus.EXPECTED,
              results: [
                {
                  result: {
                    name: `invocations/build-5678/tests/test-id/results/2`,
                    summaryHtml: `Result #2 from ${req.invocation}`,
                    expected: true,
                    status: TestStatus.PASS,
                  },
                },
              ],
            },
          ],
        }),
      );
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  it('should expand the first verdict entry when clicking on the verdict status icon', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getByText('warning'));
    await act(() => jest.runAllTimersAsync());
    expect(screen.getByText('Result #1 from invocations/inv1')).toBeVisible();
  });

  it('should not expand the first verdict entry when clicking on expand icon', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    fireEvent.click(screen.getAllByTestId('ChevronRightIcon')[1]);
    await act(() => jest.runAllTimersAsync());

    expect(
      screen.queryByText('Result #1 from invocations/inv1') ||
        document.createElement('div'),
    ).not.toBeVisible();
    expect(screen.queryByText('Changed files: 1')).toBeVisible();
  });

  it('should reset focus target when switching to another expansion button', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    // Expand entry using the verdict status column.
    fireEvent.click(screen.getByText('warning'));
    await act(() => jest.runAllTimersAsync());
    expect(screen.getByText('Result #1 from invocations/inv1')).toBeVisible();

    // Collapse entry.
    fireEvent.click(screen.getByText('warning'));
    await act(() => jest.runAllTimersAsync());
    expect(
      screen.getByText('Result #1 from invocations/inv1') ||
        document.createElement('div'),
    ).not.toBeVisible();
    expect(screen.queryByText('Changed files: 1')).not.toBeVisible();

    // Expand entry using the toggle column.
    fireEvent.click(screen.getAllByTestId('ChevronRightIcon')[1]);
    await act(() => jest.runAllTimersAsync());
    expect(
      screen.queryByText('Result #1 from invocations/inv1') ||
        document.createElement('div'),
    ).not.toBeVisible();
    expect(screen.queryByText('Changed files: 1')).toBeVisible();
  });
});
