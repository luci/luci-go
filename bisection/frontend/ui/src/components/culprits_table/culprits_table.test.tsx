// Copyright 2022 The LUCI Authors.
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

import '@testing-library/jest-dom';
import { render, screen } from '@testing-library/react';

import { CulpritsTable } from './culprits_table';

import { Culprit, CulpritAction } from '../../services/luci_bisection';

describe('Test CulpritsTable component', () => {
  test('if culprit review link is displayed', async () => {
    const mockCulprit: Culprit = {
      commit: {
        host: 'testHost',
        project: 'testProject',
        ref: 'test/ref/dev',
        id: 'ghi789ghi789',
        position: '523',
      },
      reviewTitle: 'Added new feature to improve testing again',
      reviewUrl:
        'https://chromium-review.googlesource.com/placeholder/+/234567',
      verificationDetails: {
        status: 'Confirmed Culprit',
      },
    };

    render(<CulpritsTable culprits={[mockCulprit]} />);

    await screen.findByText('Culprit CL');

    // Check there is a link to the culprit's code review
    const culpritReviewLink = screen.getByRole('link');
    expect(culpritReviewLink).toBeInTheDocument();
    expect(culpritReviewLink.getAttribute('href')).toBe(mockCulprit.reviewUrl);
    if (mockCulprit.reviewTitle) {
      expect(culpritReviewLink.textContent).toContain(mockCulprit.reviewTitle);
    }

    // Check an appropriate message is displayed for no culprit actions
    expect(
      screen.queryByText(new RegExp('no actions', 'i'))
    ).toBeInTheDocument();
  });

  test('if culprit actions are displayed', async () => {
    const bugAction: CulpritAction = {
      actionType: 'BUG_COMMENTED',
      bugUrl: 'https://crbug.com/testProject/11223344',
    };
    const autoRevertAction: CulpritAction = {
      actionType: 'CULPRIT_AUTO_REVERTED',
      revertClUrl:
        'https://chromium-review.googlesource.com/placeholder/+/123457',
    };
    const mockCulprit: Culprit = {
      commit: {
        host: 'testHost',
        project: 'testProject',
        ref: 'test/ref/dev',
        id: 'abc123abc123',
        position: '307',
      },
      reviewTitle: 'Added new feature to improve testing',
      reviewUrl:
        'https://chromium-review.googlesource.com/placeholder/+/123456',
      culpritAction: [bugAction, autoRevertAction],
      verificationDetails: {
        status: 'Confirmed Culprit',
      },
    };

    render(<CulpritsTable culprits={[mockCulprit]} />);

    await screen.findByText('Culprit CL');

    // Check there is no misleading message about culprit actions
    expect(
      screen.queryByText(new RegExp('no actions', 'i'))
    ).not.toBeInTheDocument();

    // Check the description and bug link are displayed
    const bugActionLabel = screen.getByText(
      'A comment was added on a related bug:',
      { exact: false }
    );
    expect(bugActionLabel).toBeInTheDocument();
    const bugActionLink = bugActionLabel.firstElementChild;
    expect(bugActionLink).toBeInTheDocument();
    expect(bugActionLink?.textContent).toBe('bug');
    expect(bugActionLink?.getAttribute('href')).toBe(bugAction.bugUrl);

    // Check the description and revert CL link are displayed
    const revertActionLabel = screen.getByText(
      'This culprit has been auto-reverted:',
      { exact: false }
    );
    expect(revertActionLabel).toBeInTheDocument();
    const revertActionLink = revertActionLabel.firstElementChild;
    expect(revertActionLink).toBeInTheDocument();
    expect(revertActionLink?.textContent).toBe('revert CL');
    expect(revertActionLink?.getAttribute('href')).toBe(
      autoRevertAction.revertClUrl
    );
  });

  test('if culprit verification details are displayed', async () => {
    const mockCulprit: Culprit = {
      commit: {
        host: 'testHost',
        project: 'testProject',
        ref: 'test/ref/dev',
        id: 'def456def456',
        position: '298',
      },
      reviewTitle: 'Added new feature as requested',
      reviewUrl: 'https://chromium-review.googlesource.com/placeholder/+/92345',
      verificationDetails: {
        status: 'Confirmed Culprit',
        suspectRerun: {
          startTime: '2022-09-06T07:13:16.398865Z',
          endTime: '2022-09-06T07:13:18.398865Z',
          bbid: '8877665544332211',
          rerunResult: {
            rerunStatus: 'FAILED',
          },
        },
        parentRerun: {
          startTime: '2022-09-06T07:16:16.398865Z',
          endTime: '2022-09-06T07:16:31.398865Z',
          bbid: '8765432187654321',
          rerunResult: {
            rerunStatus: 'PASSED',
          },
        },
      },
    };

    render(<CulpritsTable culprits={[mockCulprit]} />);

    await screen.findByText('Culprit CL');

    // Check the culprit's verification build numbers are displayed
    expect(
      screen.getByText(
        mockCulprit.verificationDetails.suspectRerun!.rerunResult.rerunStatus
      )
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        mockCulprit.verificationDetails.parentRerun!.rerunResult.rerunStatus
      )
    ).toBeInTheDocument();
  });

  test('if an appropriate message is displayed for no culprits', async () => {
    render(<CulpritsTable culprits={[]} />);

    await screen.findByText('Culprit CL');

    expect(screen.queryAllByTestId('culprit_table_row')).toHaveLength(0);
    expect(screen.getByText('No culprit found')).toBeInTheDocument();
  });
});
