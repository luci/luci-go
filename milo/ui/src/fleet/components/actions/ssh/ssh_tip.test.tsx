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
import { act } from 'react';

import { useBot } from '@/fleet/hooks/swarming_hooks';
import { BotInfo } from '@/proto/go.chromium.org/luci/swarming/proto/api_v2/swarming.pb';
import { useBotsClient } from '@/swarming/hooks/prpc_clients';

import { SshTip } from './ssh_tip';

jest.mock('@/fleet/hooks/swarming_hooks');
const useBotMock = useBot as jest.Mock;

jest.mock('@/swarming/hooks/prpc_clients');
const useBotsClientMock = useBotsClient as jest.Mock;

describe('<SshTip />', () => {
  beforeEach(() => {
    useBotsClientMock.mockReturnValue(undefined);
  });

  it('renders CLI command for non-satlab device', async () => {
    useBotMock.mockReturnValue({
      info: undefined,
    });
    render(<SshTip hostname="test-device" dutId="test-dut" />);

    // Open the dialog.
    await act(() => {
      const button = screen.getByRole('button');
      button.click();
    });

    expect(screen.getByText('$ ssh test-device')).toBeVisible();
  });

  it('renders CLI command for satlab device with drone server', async () => {
    useBotMock.mockReturnValue({
      info: {
        dimensions: [
          { key: 'ufs_zone', value: ['ZONE_SATLAB'] },
          { key: 'drone_server', value: ['drone.server'] },
        ],
      } as Partial<BotInfo>,
    });
    render(<SshTip hostname="satlab-device" dutId="satlab-dut" />);

    // Open the dialog.
    await act(() => {
      const button = screen.getByRole('button');
      button.click();
    });

    expect(
      screen.getByText(
        '$ ssh -o ProxyJump=moblab@drone.server root@satlab-device',
      ),
    ).toBeVisible();
  });

  it('renders CLI command and warning for satlab device without drone server', async () => {
    useBotMock.mockReturnValue({
      info: {
        dimensions: [{ key: 'ufs_zone', value: ['ZONE_SATLAB'] }],
      } as Partial<BotInfo>,
    });
    render(<SshTip hostname="satlab-device" dutId="satlab-dut" />);

    // Open the dialog.
    await act(() => {
      const button = screen.getByRole('button');
      button.click();
    });

    expect(screen.getByText('$ ssh satlab-device')).toBeVisible();
    expect(
      screen.getByText(
        'This device is a Satlab device, but we were unable to determine the' +
          ' drone server. The following SSH instructions may be incorrect.',
      ),
    ).toBeVisible();
  });

  it('renders CLI command and warning for satlab device with no bot info', async () => {
    useBotMock.mockReturnValue({
      info: undefined,
    });
    render(<SshTip hostname="satlab-device" dutId="satlab-dut" />);

    // Open the dialog.
    await act(() => {
      const button = screen.getByRole('button');
      button.click();
    });

    expect(screen.getByText('$ ssh satlab-device')).toBeVisible();
    expect(
      screen.getByText(
        'This device is a Satlab device, but we were unable to determine the' +
          ' drone server. The following SSH instructions may be incorrect.',
      ),
    ).toBeVisible();
  });
});
