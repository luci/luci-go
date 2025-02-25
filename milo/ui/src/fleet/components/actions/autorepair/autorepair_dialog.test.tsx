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

import { getByRole, render, screen } from '@testing-library/react';

import AutorepairDialog, { AutorepairDialogProps } from './autorepair_dialog';

describe('<AutorepairDialog />', () => {
  let handleCloseMock: jest.Mock;
  let handleOkMock: jest.Mock;
  let handleDeepRepairChangeMock: jest.Mock;
  let sharedTestProps: AutorepairDialogProps = {
    open: true,
    handleClose: () => undefined,
    handleOk: () => undefined,
    deepRepair: false,
    handleDeepRepairChange: () => undefined,
    sessionInfo: {},
  };

  beforeEach(() => {
    handleCloseMock = jest.fn();
    handleOkMock = jest.fn();
    handleDeepRepairChangeMock = jest.fn();

    sharedTestProps = {
      open: true,
      handleClose: handleCloseMock,
      handleOk: handleOkMock,
      deepRepair: false,
      handleDeepRepairChange: handleDeepRepairChangeMock,
      sessionInfo: {},
    };
  });

  it('renders confirmation', async () => {
    render(
      <AutorepairDialog
        {...sharedTestProps}
        sessionInfo={{ dutNames: ['test-dut'] }}
      />,
    );

    const guidance = screen.getByText(
      'Please confirm that you want to run autorepair on the following device:',
    );

    expect(guidance).toBeVisible();

    const dutLink = screen.getByRole('link', { name: 'test-dut' });
    expect(dutLink).toBeVisible();
    expect(dutLink).toHaveAttribute('href', '/ui/fleet/labs/devices/test-dut');
  });

  it('renders shivas command', async () => {
    render(
      <AutorepairDialog
        {...sharedTestProps}
        sessionInfo={{ dutNames: ['test-dut', 'dut1', 'dut2', 'dut3'] }}
      />,
    );

    const shivas = screen.getByText('$ shivas repair test-dut dut1 dut2 dut3');

    expect(shivas).toBeVisible();
  });

  it('confirms on click', async () => {
    render(
      <AutorepairDialog
        {...sharedTestProps}
        sessionInfo={{ dutNames: ['test-dut'] }}
      />,
    );

    const confirm = screen.getByRole('button', { name: 'Confirm' });
    confirm.click();

    expect(handleOkMock).toHaveBeenCalled();
    expect(handleCloseMock).not.toHaveBeenCalled();
  });

  it('renders completion step', async () => {
    render(
      <AutorepairDialog
        {...sharedTestProps}
        sessionInfo={{
          dutNames: ['test-dut'],
          sessionId: 'fake-session-info',
          builds: [
            {
              project: 'proj',
              bucket: 'buck',
              builder: 'builder',
              buildId: '1337',
            },
          ],
        }}
      />,
    );

    const text = screen.getByText(
      'Autorepair has been triggered on the following device:',
    );
    expect(text).toBeVisible();

    const dutLink = screen.getByRole('link', { name: 'test-dut' });
    expect(dutLink).toHaveAttribute('href', '/ui/fleet/labs/devices/test-dut');

    const miloLink = screen.getByRole('link', { name: 'View in Milo' });

    expect(miloLink).toHaveAttribute(
      'href',
      expect.stringContaining('/p/proj/builders/buck/builder/b1337'),
    );
  });

  it('confirm button not visible if only invalid DUTs selected', async () => {
    render(
      <AutorepairDialog
        {...sharedTestProps}
        sessionInfo={{
          invalidDutNames: ['invalid-test-dut1', 'invalid-test-dut2'],
        }}
      />,
    );

    expect(
      screen.queryByRole('button', { name: 'Confirm' }),
    ).not.toBeInTheDocument();
  });

  it('warning displayed when mix of valid and invalid DUTs selected', async () => {
    const invalidDuts = ['invalid-test-dut1', 'invalid-test-dut2'];
    render(
      <AutorepairDialog
        {...sharedTestProps}
        sessionInfo={{
          dutNames: ['test-dut1', 'test-dut2'],
          invalidDutNames: invalidDuts,
        }}
      />,
    );

    expect(
      screen.queryByRole('button', { name: 'Confirm' }),
    ).toBeInTheDocument();

    const warning = screen.getByText(
      /For the following devices autorepair will not be executed/,
    );

    expect(warning).toBeVisible();

    invalidDuts.forEach((dutName) =>
      expect(getByRole(warning, 'link', { name: dutName })).toBeVisible(),
    );
  });
});
