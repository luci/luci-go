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

import { render, screen } from '@testing-library/react';

import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import DeployDialog, { DeployDialogProps } from './deploy_dialog';

describe('<DeployDialog />', () => {
  let handleCloseMock: jest.Mock;
  let handleOkMock: jest.Mock;
  let sharedTestProps: DeployDialogProps;

  beforeEach(() => {
    handleCloseMock = jest.fn();
    handleOkMock = jest.fn();

    sharedTestProps = {
      open: true,
      handleClose: handleCloseMock,
      handleOk: handleOkMock,
      sessionInfo: {},
      loading: false,
    };
  });

  it('renders confirmation', async () => {
    render(
      <FakeAuthStateProvider>
        <DeployDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['test-dut'] }}
        />
      </FakeAuthStateProvider>,
    );

    const guidance = screen.getByText(
      'Please confirm that you want to deploy the following device:',
    );
    expect(guidance).toBeVisible();

    const dutLink = screen.getByRole('link', { name: 'test-dut' });
    expect(dutLink).toBeVisible();
    expect(dutLink).toHaveAttribute(
      'href',
      '/ui/fleet/p/chromeos/devices/test-dut',
    );
  });

  it('renders shivas command', async () => {
    render(
      <FakeAuthStateProvider>
        <DeployDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['test-dut', 'dut1', 'dut2', 'dut3'] }}
        />
      </FakeAuthStateProvider>,
    );

    const shivas = screen.getByText(
      'shivas update dut -force-deploy test-dut dut1 dut2 dut3',
    );
    expect(shivas).toBeVisible();
  });

  it('renders loading spinner', async () => {
    render(
      <FakeAuthStateProvider>
        <DeployDialog
          {...sharedTestProps}
          loading={true}
          sessionInfo={{ dutNames: ['test-dut'] }}
        />
      </FakeAuthStateProvider>,
    );
    expect(screen.getByRole('progressbar')).toBeVisible();
  });

  it('confirms on click', async () => {
    render(
      <FakeAuthStateProvider>
        <DeployDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['test-dut'] }}
        />
      </FakeAuthStateProvider>,
    );

    const confirm = screen.getByRole('button', { name: 'Confirm' });
    confirm.click();

    expect(handleOkMock).toHaveBeenCalled();
    expect(handleCloseMock).not.toHaveBeenCalled();
  });

  it('renders completion step', async () => {
    render(
      <FakeAuthStateProvider>
        <DeployDialog
          {...sharedTestProps}
          sessionInfo={{
            results: [
              {
                unitName: 'test-dut',
                taskUrl: '/p/proj/builders/buck/builder/b1337',
              },
            ],
            sessionId: 'fake-session-info',
          }}
        />
      </FakeAuthStateProvider>,
    );

    const text = screen.getByText(
      'Deploy has been triggered on the following device:',
    );
    expect(text).toBeVisible();

    const dutLink = screen.getByRole('link', { name: 'test-dut' });
    expect(dutLink).toHaveAttribute(
      'href',
      '/ui/fleet/p/chromeos/devices/test-dut',
    );

    const miloLink = screen.getByRole('link', { name: 'View in Milo' });
    expect(miloLink).toHaveAttribute(
      'href',
      '/p/proj/builders/buck/builder/b1337',
    );
  });

  it('displays error for failed deploy', async () => {
    render(
      <FakeAuthStateProvider>
        <DeployDialog
          {...sharedTestProps}
          sessionInfo={{
            results: [
              {
                unitName: 'test-dut-1',
                taskUrl: '/p/proj/builders/buck/builder/b1337',
              },
              {
                unitName: 'test-dut-2',
                errorMessage: 'it broke',
              },
            ],
          }}
        />
      </FakeAuthStateProvider>,
    );

    expect(screen.getByRole('link', { name: 'View in Milo' })).toBeVisible();
    expect(
      screen.getByText('Failed to schedule deploy: it broke'),
    ).toBeVisible();
  });

  it('renders external partner message and disables confirm button', async () => {
    render(
      <FakeAuthStateProvider>
        <DeployDialog
          {...sharedTestProps}
          sessionInfo={{
            dutNames: ['partner-dut'],
            namespaces: ['os-partner'],
          }}
        />
      </FakeAuthStateProvider>,
    );

    expect(
      screen.getByText(
        'At this time, the Fleet Console does not support force deploy for external devices.',
      ),
    ).toBeVisible();

    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmButton).toBeDisabled();
  });
});
