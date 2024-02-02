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

import { render, screen } from '@testing-library/react';

import { GenericCulprit } from '@/bisection/types';
import { RerunStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import {
  Culprit,
  CulpritAction,
  CulpritActionType,
  CulpritInactionReason,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/culprits.pb';

import { CulpritsTable } from './culprits_table';

describe('<CulpritsTable />', () => {
  test('if culprit review link is displayed', async () => {
    const mockCulprit = GenericCulprit.from(
      Culprit.fromPartial({
        commit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'ghi789ghi789',
          position: 523,
        },
        reviewTitle: 'Added new feature to improve testing again',
        reviewUrl:
          'https://chromium-review.googlesource.com/placeholder/+/234567',
        verificationDetails: {
          status: 'Confirmed Culprit',
        },
      }),
    );

    render(<CulpritsTable culprits={[mockCulprit]} />);

    await screen.findByTestId('culprits-table');

    // Check there is a link to the culprit's code review
    const culpritReviewLink = screen.getByRole('link');
    expect(culpritReviewLink).toBeInTheDocument();
    expect(culpritReviewLink.getAttribute('href')).toBe(mockCulprit.reviewUrl);
    expect(culpritReviewLink.textContent).toContain(mockCulprit.reviewTitle);

    // Check an appropriate message is displayed for no culprit actions
    expect(screen.getByText(new RegExp('no actions', 'i'))).toBeInTheDocument();
  });

  test('if culprit actions are displayed', async () => {
    const bugAction = CulpritAction.fromPartial({
      actionType: CulpritActionType.BUG_COMMENTED,
      bugUrl: 'https://crbug.com/testProject/11223344',
    });
    const autoRevertAction = CulpritAction.fromPartial({
      actionType: CulpritActionType.CULPRIT_AUTO_REVERTED,
      revertClUrl:
        'https://chromium-review.googlesource.com/placeholder/+/123457',
    });
    const mockCulprit = GenericCulprit.from(
      Culprit.fromPartial({
        commit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'abc123abc123',
          position: 307,
        },
        reviewTitle: 'Added new feature to improve testing',
        reviewUrl:
          'https://chromium-review.googlesource.com/placeholder/+/123456',
        culpritAction: Object.freeze([bugAction, autoRevertAction]),
        verificationDetails: {
          status: 'Confirmed Culprit',
        },
      }),
    );

    render(<CulpritsTable culprits={[mockCulprit]} />);

    await screen.findByTestId('culprits-table');

    // Check there is no misleading message about culprit actions
    expect(
      screen.queryByText(new RegExp('no actions', 'i')),
    ).not.toBeInTheDocument();

    // Check the description and bug link are displayed
    const bugActionLabel = screen.getByText(
      'A comment was added on a related bug:',
      { exact: false },
    );
    expect(bugActionLabel).toBeInTheDocument();
    const bugActionLink = bugActionLabel.firstElementChild;
    expect(bugActionLink).toBeInTheDocument();
    expect(bugActionLink?.textContent).toBe('bug');
    expect(bugActionLink?.getAttribute('href')).toBe(bugAction.bugUrl);

    // Check the description and revert CL link are displayed
    const revertActionLabel = screen.getByText(
      'This culprit has been auto-reverted:',
      { exact: false },
    );
    expect(revertActionLabel).toBeInTheDocument();
    const revertActionLink = revertActionLabel.firstElementChild;
    expect(revertActionLink).toBeInTheDocument();
    expect(revertActionLink?.textContent).toBe('revert CL');
    expect(revertActionLink?.getAttribute('href')).toBe(
      autoRevertAction.revertClUrl,
    );
  });

  test('if culprit inaction with reason is displayed', async () => {
    const inaction = CulpritAction.fromPartial({
      actionType: CulpritActionType.NO_ACTION,
      inactionReason: CulpritInactionReason.REVERTED_BY_BISECTION,
      revertClUrl:
        'https://chromium-review.googlesource.com/placeholder/+/123457',
    });
    const mockCulprit = GenericCulprit.from(
      Culprit.fromPartial({
        commit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'abc123abc123',
          position: 307,
        },
        reviewTitle: 'Added new feature to improve testing',
        reviewUrl:
          'https://chromium-review.googlesource.com/placeholder/+/123456',
        culpritAction: Object.freeze([inaction]),
        verificationDetails: {
          status: 'Confirmed Culprit',
        },
      }),
    );

    render(<CulpritsTable culprits={[mockCulprit]} />);

    await screen.findByTestId('culprits-table');

    // Check the description and inaction reason are displayed
    const inactionLabel = screen.getByText(
      'No actions have been performed by LUCI Bisection for this culprit',
      { exact: false },
    );
    expect(inactionLabel).toBeInTheDocument();
    expect(inactionLabel.textContent).toMatch(
      'it has been reverted as the culprit of another LUCI Bisection analysis',
    );
    const inactionRevertLink = inactionLabel.firstElementChild;
    expect(inactionRevertLink).toBeInTheDocument();
    expect(inactionRevertLink?.textContent).toBe('revert CL');
    expect(inactionRevertLink?.getAttribute('href')).toBe(inaction.revertClUrl);
  });

  test('if culprit inaction without reason is displayed', async () => {
    const inaction = CulpritAction.fromPartial({
      actionType: CulpritActionType.NO_ACTION,
    });
    const mockCulprit = GenericCulprit.from(
      Culprit.fromPartial({
        commit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'abc123abc123',
          position: 307,
        },
        reviewTitle: 'Added new feature to improve testing',
        reviewUrl:
          'https://chromium-review.googlesource.com/placeholder/+/123456',
        culpritAction: Object.freeze([inaction]),
        verificationDetails: {
          status: 'Confirmed Culprit',
        },
      }),
    );

    render(<CulpritsTable culprits={[mockCulprit]} />);

    await screen.findByTestId('culprits-table');

    // Check the description and inaction reason are displayed
    const inactionLabel = screen.getByText(
      'No actions have been performed by LUCI Bisection for this culprit.',
      { exact: true },
    );
    expect(inactionLabel).toBeInTheDocument();
  });

  test('if culprit verification details are displayed', async () => {
    const mockCulprit = GenericCulprit.from(
      Culprit.fromPartial({
        commit: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'def456def456',
          position: 298,
        },
        reviewTitle: 'Added new feature as requested',
        reviewUrl:
          'https://chromium-review.googlesource.com/placeholder/+/92345',
        verificationDetails: {
          status: 'Confirmed Culprit',
          suspectRerun: {
            startTime: '2022-09-06T07:13:16.398865Z',
            endTime: '2022-09-06T07:13:18.398865Z',
            bbid: '8877665544332211',
            rerunResult: {
              rerunStatus: RerunStatus.FAILED,
            },
            commit: {
              host: 'testHost',
              project: 'testProject',
              ref: 'test/ref/dev',
              id: 'def456def456',
            },
            type: 'Culprit Verification',
          },
          parentRerun: {
            startTime: '2022-09-06T07:16:16.398865Z',
            endTime: '2022-09-06T07:16:31.398865Z',
            bbid: '8765432187654321',
            rerunResult: {
              rerunStatus: RerunStatus.PASSED,
            },
            commit: {
              host: 'testHost',
              project: 'testProject',
              ref: 'test/ref/dev',
              id: 'def456def455',
            },
            type: 'Culprit Verification',
          },
        },
      }),
    );

    render(<CulpritsTable culprits={[mockCulprit]} />);

    await screen.findByTestId('culprits-table');

    // Check the culprit's verification build results are displayed
    expect(screen.getByText('Failed')).toBeInTheDocument();
    expect(screen.getByText('Passed')).toBeInTheDocument();
  });

  test('if an appropriate message is displayed for no culprits', async () => {
    render(<CulpritsTable culprits={[]} />);

    await screen.findByTestId('culprits-table');

    expect(screen.queryAllByTestId('culprit_table_row')).toHaveLength(0);
    expect(screen.getByText('No culprit found')).toBeInTheDocument();
  });
});
