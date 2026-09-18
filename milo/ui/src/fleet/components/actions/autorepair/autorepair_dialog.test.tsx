// Copyright 2025 The LUCI Authors.
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
import userEvent from '@testing-library/user-event';

import { FLEET_BUILDS_SWARMING_HOST } from '@/fleet/utils/builds';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import AutorepairDialog, { AutorepairDialogProps } from './autorepair_dialog';
import { generateAutorepairBugMarkdown } from './autorepair_utils';

describe('<AutorepairDialog />', () => {
  let handleCloseMock: jest.Mock;
  let handleOkMock: jest.Mock;
  let handleDeepRepairChangeMock: jest.Mock;
  let handleLatestRepairChangeMock: jest.Mock;
  let handleVerifyOnlyChangeMock: jest.Mock;
  let sharedTestProps: AutorepairDialogProps = {
    open: true,
    handleClose: () => undefined,
    handleOk: () => undefined,
    deepRepair: false,
    handleDeepRepairChange: () => undefined,
    latestRepair: false,
    handleLatestRepairChange: () => undefined,
    verifyOnly: false,
    handleVerifyOnlyChange: () => undefined,
    sessionInfo: {},
    loading: false,
  };

  beforeEach(() => {
    handleCloseMock = jest.fn();
    handleOkMock = jest.fn();
    handleDeepRepairChangeMock = jest.fn();
    handleLatestRepairChangeMock = jest.fn();
    handleVerifyOnlyChangeMock = jest.fn();

    sharedTestProps = {
      open: true,
      handleClose: handleCloseMock,
      handleOk: handleOkMock,
      deepRepair: false,
      handleDeepRepairChange: handleDeepRepairChangeMock,
      latestRepair: false,
      handleLatestRepairChange: handleLatestRepairChangeMock,
      verifyOnly: false,
      handleVerifyOnlyChange: handleVerifyOnlyChangeMock,
      sessionInfo: {},
      loading: false,
    };
  });

  it('renders confirmation', async () => {
    render(
      <FakeAuthStateProvider>
        <AutorepairDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['test-dut'] }}
        />
      </FakeAuthStateProvider>,
    );

    const guidance = screen.getByText(
      'Please confirm that you want to run autorepair on the following device:',
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
        <AutorepairDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['test-dut', 'dut1', 'dut2', 'dut3'] }}
        />
      </FakeAuthStateProvider>,
    );

    const shivas = screen.getByText('shivas repair test-dut dut1 dut2 dut3');

    expect(shivas).toBeVisible();
  });

  it('renders satlab command for partner devices', async () => {
    render(
      <FakeAuthStateProvider>
        <AutorepairDialog
          {...sharedTestProps}
          sessionInfo={{
            dutNames: ['test-dut', 'dut1', 'dut2', 'dut3'],
            namespaces: ['os-partner'],
          }}
        />
      </FakeAuthStateProvider>,
    );

    const satlab = screen.getByText(
      'satlab repair dut test-dut dut1 dut2 dut3',
    );
    expect(satlab).toBeVisible();

    expect(
      screen.getByText(
        /Autorepair via Fleet Console is not currently supported/i,
      ),
    ).toBeVisible();

    const confirmButton = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmButton).toBeDisabled();
  });

  it('renders all checkboxes', () => {
    render(
      <FakeAuthStateProvider>
        <AutorepairDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['test-dut'] }}
        />
      </FakeAuthStateProvider>,
    );
    expect(
      screen.getByRole('checkbox', { name: 'Deep repair these devices' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('checkbox', { name: 'Use latest repair version' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('checkbox', { name: 'Verify Only' }),
    ).toBeInTheDocument();
  });

  it('shows warning when both deep and verify are checked', () => {
    const { rerender } = render(
      <FakeAuthStateProvider>
        <AutorepairDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['test-dut'] }}
          deepRepair={false}
          verifyOnly={false}
        />
      </FakeAuthStateProvider>,
    );

    expect(
      screen.queryByText('Unusual case: both deep and verify are checked'),
    ).not.toBeInTheDocument();

    rerender(
      <FakeAuthStateProvider>
        <AutorepairDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['test-dut'] }}
          deepRepair={true}
          verifyOnly={true}
        />
      </FakeAuthStateProvider>,
    );

    const warning = screen.getByText(
      'Unusual case: both deep and verify are checked. Verify build will be used but task will trigger a full deep recovery workflow.',
    );
    expect(warning).toBeVisible();
  });

  it('calls the verify only handler when clicked', () => {
    render(
      <FakeAuthStateProvider>
        <AutorepairDialog
          {...sharedTestProps}
          sessionInfo={{ dutNames: ['test-dut'] }}
        />
      </FakeAuthStateProvider>,
    );

    const verifyOnlyCheckbox = screen.getByRole('checkbox', {
      name: 'Verify Only',
    });

    fireEvent.click(verifyOnlyCheckbox);

    expect(handleVerifyOnlyChangeMock).toHaveBeenCalledWith(true);
  });

  it('renders loading spinner', async () => {
    render(
      <FakeAuthStateProvider>
        <AutorepairDialog
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
        <AutorepairDialog
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
        <AutorepairDialog
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
      'Autorepair has been triggered on the following device:',
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

  it('displays error for failed autorepair', async () => {
    render(
      <FakeAuthStateProvider>
        <AutorepairDialog
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
      screen.getByText('Failed to schedule autorepair: it broke'),
    ).toBeVisible();
  });

  it('renders collapsible bug markdown snippet collapsed by default and copies on button click', async () => {
    const user = userEvent.setup();

    render(
      <FakeAuthStateProvider>
        <AutorepairDialog
          {...sharedTestProps}
          sessionInfo={{
            results: [
              {
                unitName: 'test-dut-1',
                taskUrl:
                  'https://ci.chromium.org/ui/p/chromeos/builders/repair/b1337',
              },
              {
                unitName: 'test-dut-2',
                errorMessage: 'device locked',
              },
            ],
            sessionId: 'session-123',
          }}
        />
      </FakeAuthStateProvider>,
    );

    const expandBtn = screen.getByRole('button', {
      name: 'Autorepair results (Markdown for Buganizer):',
    });
    expect(expandBtn).toBeVisible();
    expect(expandBtn).toHaveAttribute('aria-expanded', 'false');

    // Expand the collapsible section
    await user.click(expandBtn);
    expect(expandBtn).toHaveAttribute('aria-expanded', 'true');

    const copyBtn = screen.getByRole('button', { name: 'Copy markdown' });
    expect(copyBtn).toBeVisible();

    await user.click(copyBtn);

    const expectedMarkdown = generateAutorepairBugMarkdown(
      [
        {
          unitName: 'test-dut-1',
          taskUrl:
            'https://ci.chromium.org/ui/p/chromeos/builders/repair/b1337',
        },
        {
          unitName: 'test-dut-2',
          errorMessage: 'device locked',
        },
      ],
      'session-123',
    );

    expect(await navigator.clipboard.readText()).toBe(expectedMarkdown);
    expect(screen.getByRole('button', { name: 'Copied' })).toBeVisible();
  });
});

describe('generateAutorepairBugMarkdown', () => {
  it('formats successful tasks, failed tasks, and swarming session link', () => {
    const md = generateAutorepairBugMarkdown(
      [
        {
          unitName: 'dut-a',
          taskUrl: 'https://ci.chromium.org/b/111',
        },
        {
          unitName: 'dut-b',
          errorMessage: 'RPC timeout',
        },
      ],
      'sess-abc',
    );

    expect(md).toContain('**Autorepair results:**');
    expect(md).toContain(
      '* [dut-a](http://localhost/ui/fleet/p/chromeos/devices/dut-a): [View in Milo](https://ci.chromium.org/b/111)',
    );
    expect(md).toContain(
      '* [dut-b](http://localhost/ui/fleet/p/chromeos/devices/dut-b): Failed to schedule autorepair: RPC timeout',
    );
    expect(md).toContain(
      `[View tasks in Swarming](https://${FLEET_BUILDS_SWARMING_HOST}/tasklist?f=admin-session:sess-abc)`,
    );
  });
});
