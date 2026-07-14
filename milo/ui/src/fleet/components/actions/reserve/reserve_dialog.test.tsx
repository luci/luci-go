// Copyright 2026 The LUCI Authors.
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

import { FLEET_BUILDS_SWARMING_HOST } from '@/fleet/utils/builds';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import ReserveDialog, { ReserveDialogProps } from './reserve_dialog';

describe('<ReserveDialog />', () => {
  let handleCloseMock: jest.Mock;
  let handleOkMock: jest.Mock;
  let handleCommentChangeMock: jest.Mock;
  let sharedTestProps: ReserveDialogProps;

  beforeEach(() => {
    handleCloseMock = jest.fn();
    handleOkMock = jest.fn();
    handleCommentChangeMock = jest.fn();

    sharedTestProps = {
      open: true,
      handleClose: handleCloseMock,
      handleOk: handleOkMock,
      loading: false,
      comment: 'Testing',
      handleCommentChange: handleCommentChangeMock,
      sessionInfo: {},
      latest: false,
      handleLatestChange: jest.fn(),
    };
  });

  it('renders confirmation screen with selected devices', async () => {
    render(
      <FakeAuthStateProvider>
        <ReserveDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['device-1', 'device-2'] }}
        />
      </FakeAuthStateProvider>,
    );

    expect(
      screen.getByText(
        'Please confirm that you want to reserve the following 2 devices:',
      ),
    ).toBeVisible();

    expect(screen.getByRole('link', { name: 'device-1' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'device-2' })).toBeVisible();
  });

  it('renders shivas command preview', async () => {
    render(
      <FakeAuthStateProvider>
        <ReserveDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['device-1'] }}
          comment="Testing"
        />
      </FakeAuthStateProvider>,
    );

    expect(
      screen.getByText('shivas reserve-duts -comment "Testing" device-1'),
    ).toBeVisible();
  });

  it('escapes double quotes in shivas command preview', async () => {
    render(
      <FakeAuthStateProvider>
        <ReserveDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['device-1'] }}
          comment='Testing "my" device'
        />
      </FakeAuthStateProvider>,
    );

    expect(
      screen.getByText(
        'shivas reserve-duts -comment "Testing \\"my\\" device" device-1',
      ),
    ).toBeVisible();
  });

  it('disables confirm button when reason is empty', async () => {
    render(
      <FakeAuthStateProvider>
        <ReserveDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['device-1'] }}
          comment=""
        />
      </FakeAuthStateProvider>,
    );

    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmButton).toBeDisabled();
  });

  it('calls handleCommentChange when typing a comment', async () => {
    render(
      <FakeAuthStateProvider>
        <ReserveDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['device-1'] }}
        />
      </FakeAuthStateProvider>,
    );

    const input = screen.getByLabelText(/Comment/i);
    fireEvent.change(input, { target: { value: 'New Reason' } });

    expect(handleCommentChangeMock).toHaveBeenCalledWith('New Reason');
  });

  it('renders loading spinner', async () => {
    render(
      <FakeAuthStateProvider>
        <ReserveDialog
          {...sharedTestProps}
          loading={true}
          sessionInfo={{ dutNames: ['device-1'] }}
        />
      </FakeAuthStateProvider>,
    );

    expect(screen.getByRole('progressbar')).toBeVisible();
  });

  it('renders success results screen with link to task and swarming task list', async () => {
    render(
      <FakeAuthStateProvider>
        <ReserveDialog
          {...sharedTestProps}
          sessionInfo={{
            sessionId: 'test-session-id',
            results: [
              {
                unitName: 'device-1',
                success: true,
                redirectUrl: 'https://milo/task-1',
              },
            ],
          }}
        />
      </FakeAuthStateProvider>,
    );

    expect(
      screen.getByText('Reserve has been triggered on the following device:'),
    ).toBeVisible();

    const link = screen.getByRole('link', { name: 'View in Milo' });
    expect(link).toBeVisible();
    expect(link).toHaveAttribute('href', 'https://milo/task-1');

    const swarmingLink = screen.getByRole('link', {
      name: 'View tasks in Swarming',
    });
    expect(swarmingLink).toBeVisible();
    expect(swarmingLink).toHaveAttribute(
      'href',
      `https://${FLEET_BUILDS_SWARMING_HOST}/tasklist?f=admin-session:test-session-id`,
    );

    expect(
      screen.getByText(
        'It may take a few minutes for Swarming to update the task to show up on Milo.',
      ),
    ).toBeVisible();
  });

  it('renders success results screen without Swarming link when sessionId is undefined', async () => {
    render(
      <FakeAuthStateProvider>
        <ReserveDialog
          {...sharedTestProps}
          sessionInfo={{
            results: [
              {
                unitName: 'device-1',
                success: true,
                redirectUrl: 'https://milo/task-1',
              },
            ],
          }}
        />
      </FakeAuthStateProvider>,
    );

    expect(
      screen.getByText('Reserve has been triggered on the following device:'),
    ).toBeVisible();

    const link = screen.getByRole('link', { name: 'View in Milo' });
    expect(link).toBeVisible();
    expect(link).toHaveAttribute('href', 'https://milo/task-1');

    const swarmingLink = screen.queryByRole('link', {
      name: 'View tasks in Swarming',
    });
    expect(swarmingLink).toBeNull();

    expect(
      screen.queryByText(
        'It may take a few minutes for Swarming to update the task to show up on Milo.',
      ),
    ).toBeNull();
  });

  it('renders failure results screen with error message', async () => {
    render(
      <FakeAuthStateProvider>
        <ReserveDialog
          {...sharedTestProps}
          sessionInfo={{
            results: [
              {
                unitName: 'bni1-labstation',
                success: false,
                errorMessage: 'Device is not eligible for reservation',
              },
            ],
          }}
        />
      </FakeAuthStateProvider>,
    );

    expect(
      screen.getByText(
        'Failed to schedule reserve: Device is not eligible for reservation',
      ),
    ).toBeVisible();
  });
});
