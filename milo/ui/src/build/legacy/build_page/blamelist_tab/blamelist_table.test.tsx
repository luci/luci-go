// Copyright 2023 The LUCI Authors.
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

import { render, screen, within } from '@testing-library/react';
import { act } from 'react';
import { MemoryRouter } from 'react-router';
import { VirtuosoMockContext } from 'react-virtuoso';

import { Analysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { Commit } from '@/proto/go.chromium.org/luci/common/proto/git/commit.pb';
import { QueryBlamelistResponse } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

import { BlamelistTable } from './blamelist_table';
import { OutputQueryBlamelistResponse } from './types';

function makeCommit(
  id: string,
  message = 'this is a commit\ndescription\n',
): Commit {
  return {
    id,
    tree: '1234567890abcdef',
    parents: ['1234567890abcdee'],
    author: {
      name: 'author',
      email: 'author@email.com',
      time: '2022-02-02T23:22:22Z',
    },
    committer: {
      name: 'committer',
      email: 'committer@email.com',
      time: '2022-02-02T23:22:22Z',
    },
    message,
    treeDiff: [],
  };
}

describe('<BlamelistTable />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('can assign the commit numbers correctly', async () => {
    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 10 }}
      >
        <BlamelistTable
          repoUrl="https://repo.url"
          pages={[
            QueryBlamelistResponse.fromPartial({
              commits: [
                makeCommit('commit1'),
                makeCommit('commit2'),
                makeCommit('commit3'),
              ],
            }) as OutputQueryBlamelistResponse,
            QueryBlamelistResponse.fromPartial({
              commits: [makeCommit('commit4'), makeCommit('commit5')],
            }) as OutputQueryBlamelistResponse,
            QueryBlamelistResponse.fromPartial({
              commits: [makeCommit('commit6')],
            }) as OutputQueryBlamelistResponse,
          ]}
          revertedCommitsMap={new Map()}
        />
      </VirtuosoMockContext.Provider>,
    );

    await act(() => jest.runAllTimersAsync());
    expect(screen.getByText('commit1').closest('tr')).toHaveTextContent('1.');
    expect(screen.getByText('commit2').closest('tr')).toHaveTextContent('2.');
    expect(screen.getByText('commit3').closest('tr')).toHaveTextContent('3.');
    expect(screen.getByText('commit4').closest('tr')).toHaveTextContent('4.');
    expect(screen.getByText('commit5').closest('tr')).toHaveTextContent('5.');
    expect(screen.getByText('commit6').closest('tr')).toHaveTextContent('6.');
    expect(screen.queryByText('Culprit Analysis')).not.toBeInTheDocument();
  });

  it('should show reverted by column', async () => {
    const revertCommit = makeCommit(
      'revert-commit-id',
      'Revert "the commit title"\n\nThis reverts commit reverted-commit-id-long.',
    );
    const revertedCommit = makeCommit(
      'reverted-commit-id-long',
      'reverted title',
    );
    const anotherCommit = makeCommit('another-commit');
    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 10 }}
      >
        <BlamelistTable
          repoUrl="https://repo.url"
          pages={[
            QueryBlamelistResponse.fromPartial({
              commits: [revertedCommit, revertCommit, anotherCommit],
            }) as OutputQueryBlamelistResponse,
          ]}
          revertedCommitsMap={
            new Map([
              [
                'reverted-commit-id-long',
                {
                  host: 'repo.url',
                  project: '',
                  id: 'revert-commit-id',
                } as GitilesCommit,
              ],
            ])
          }
        />
      </VirtuosoMockContext.Provider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(screen.getByText('Reverted By')).toBeInTheDocument();

    const revertedRow = screen
      .getByRole('link', { name: 'reverted' })
      .closest('tr')!;
    const revertingLink = within(revertedRow).getByRole('link', {
      name: 'revert-',
    });
    expect(revertingLink).toHaveAttribute(
      'href',
      'https://repo.url/+/revert-commit-id',
    );
    const titleCell = within(revertedRow).getByText('reverted title');
    expect(titleCell).toHaveStyle('text-decoration: line-through');
    expect(revertedRow).not.toHaveStyle('text-decoration: line-through');

    const revertRow = screen
      .getByText('Revert "the commit title"')
      .closest('tr')!;
    // It should have only one link, which is the commit ID link.
    expect(within(revertRow).getAllByRole('link')).toHaveLength(1);
    expect(revertRow).not.toHaveStyle('text-decoration: line-through');

    const anotherRow = screen
      .getByRole('link', { name: 'another-' })
      .closest('tr')!;
    // It should have only one link, which is the commit ID link.
    expect(within(anotherRow).getAllByRole('link')).toHaveLength(1);
    expect(anotherRow).not.toHaveStyle('text-decoration: line-through');
  });

  it('displays the Culprit Analysis column', async () => {
    render(
      <MemoryRouter>
        <VirtuosoMockContext.Provider
          value={{ viewportHeight: 300, itemHeight: 10 }}
        >
          <BlamelistTable
            repoUrl="https://repo.url"
            pages={[
              QueryBlamelistResponse.fromPartial({
                commits: [
                  makeCommit('commit1'),
                  makeCommit('commit2'),
                  makeCommit('commit3'),
                  makeCommit('badcommit'),
                ],
              }) as OutputQueryBlamelistResponse,
            ]}
            analysis={Analysis.fromPartial({
              firstFailedBbid: '0123456789',
              genAiResult: {
                suspect: {
                  commit: {
                    id: 'badcommit',
                  },
                  verificationDetails: {
                    status: 'Under Verification',
                  },
                },
              },
            })}
            revertedCommitsMap={new Map()}
          />
        </VirtuosoMockContext.Provider>
      </MemoryRouter>,
    );

    await act(() => jest.runAllTimersAsync());
    expect(screen.getByText('Culprit Analysis')).toBeInTheDocument();
    expect(screen.queryByTestId('culprit')).toHaveTextContent('Suspect');
    expect(screen.getByText('commit1').closest('tr')).toHaveTextContent('1.');
    expect(screen.getByText('commit2').closest('tr')).toHaveTextContent('2.');
    expect(screen.getByText('commit3').closest('tr')).toHaveTextContent('3.');
  });
});
