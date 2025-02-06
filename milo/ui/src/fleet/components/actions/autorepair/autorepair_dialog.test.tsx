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

import { render, screen } from '@testing-library/react';

import AutorepairDialog from './autorepair_dialog';

describe('<AutorepairDialog />', () => {
  let handleCloseMock: jest.Mock;
  let handleOkMock: jest.Mock;

  beforeEach(() => {
    handleCloseMock = jest.fn();
    handleOkMock = jest.fn();
  });

  it('renders confirmation', async () => {
    render(
      <AutorepairDialog
        open={true}
        handleClose={handleCloseMock}
        handleOk={handleOkMock}
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
        open={true}
        handleClose={handleCloseMock}
        handleOk={handleOkMock}
        sessionInfo={{ dutNames: ['test-dut', 'dut1', 'dut2', 'dut3'] }}
      />,
    );

    const shivas = screen.getByText('$ shivas repair test-dut dut1 dut2 dut3');

    expect(shivas).toBeVisible();
  });

  it('confirms on click', async () => {
    render(
      <AutorepairDialog
        open={true}
        handleClose={handleCloseMock}
        handleOk={handleOkMock}
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
        open={true}
        handleClose={handleCloseMock}
        handleOk={handleOkMock}
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
});
