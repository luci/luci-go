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

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { generateServoHostnameFilterURL } from '@/fleet/config/chromeos_device_config';
import { SettingsProvider } from '@/fleet/context/providers';
import {
  ChromeOSDevice,
  getFieldDefinition,
} from '@/fleet/pages/device_list_page/chromeos/chromeos_fields';
import { FC_CellProps } from '@/fleet/types/table';
import {
  DeviceState,
  DeviceType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSDeviceDetailsPage } from './chromeos_device_details_page';
import { ChromeOSDeviceDimensions } from './chromeos_device_dimensions';
import { CodeChip } from './components/common/CodeChip';
import { useChromeOSDeviceData } from './use_chromeos_device_data';

jest.mock('./use_chromeos_device_data');
jest.mock('@/fleet/pages/device_details_page/common/bot_information', () => ({
  BotInformation: () => <div data-testid="bot-info" />,
}));
jest.mock('@/fleet/pages/device_details_page/common/bot_state', () => ({
  BotState: () => <div data-testid="bot-state" />,
}));

const mockUseChromeOSDeviceData = useChromeOSDeviceData as jest.Mock;

describe('<ChromeOSDeviceDetailsPage />', () => {
  afterEach(() => {
    localStorage.clear();
  });

  it('renders loading by default', async () => {
    mockUseChromeOSDeviceData.mockReturnValue({ isLoading: true });
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <ShortcutProvider>
            <ChromeOSDeviceDetailsPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('loading-spinner')).toBeVisible();
  });

  it('renders servo_hostname as a 1-click filter link', () => {
    const def = getFieldDefinition('servo_hostname');
    const cell = def.renderCell!({
      cell: { getValue: () => ['host-1'] },
      row: { original: { id: 'chromeos6-row1-rack2-host3' } as ChromeOSDevice },
      column: { id: 'servo_hostname' },
    } as unknown as FC_CellProps<ChromeOSDevice>);
    render(<FakeContextProvider>{cell}</FakeContextProvider>);
    expect(screen.getByRole('link', { name: 'host-1' })).toHaveAttribute(
      'href',
      generateServoHostnameFilterURL('host-1', 'servo_hostname'),
    );
  });

  it('renders CodeChip with 1-click copy button and optional link', () => {
    render(<CodeChip value="brya" href="http://go/dlm-board/brya" />);
    expect(screen.getByRole('link', { name: 'brya' })).toHaveAttribute(
      'href',
      'http://go/dlm-board/brya',
    );
    expect(screen.getByRole('button', { name: 'Copy brya' })).toBeVisible();
  });

  it('pins priority triage labels above alphabetical labels and filters with fuzzySubstring', () => {
    const mockDevice: ChromeOSDevice = {
      id: 'chromeos6-row1-rack2-host3',
      dutId: 'C123456',
      address: undefined,
      type: DeviceType.DEVICE_TYPE_UNSPECIFIED,
      state: DeviceState.DEVICE_STATE_AVAILABLE,
      realm: 'chromeos:fleet',
      deviceSpec: {
        labels: {
          'zebra-label': { values: ['z-val'] },
          'alpha-label': { values: ['a-val'] },
          'label-board': { values: ['brya'] },
          'label-servo_hostname': {
            values: ['chromeos6-row1-rack2-labstation1'],
          },
        },
      },
    };

    render(
      <FakeContextProvider>
        <SettingsProvider>
          <ChromeOSDeviceDimensions device={mockDevice} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const rows = screen.getAllByRole('row');
    const rowTexts = rows.map((r) => r.textContent ?? '');
    const servoIdx = rowTexts.findIndex((t) =>
      t.includes('label-servo_hostname'),
    );
    const boardIdx = rowTexts.findIndex((t) => t.includes('label-board'));
    const alphaIdx = rowTexts.findIndex((t) => t.includes('alpha-label'));
    const zebraIdx = rowTexts.findIndex((t) => t.includes('zebra-label'));

    expect(servoIdx).toBeGreaterThan(0);
    expect(servoIdx).toBeLessThan(boardIdx);
    expect(boardIdx).toBeLessThan(alphaIdx);
    expect(alphaIdx).toBeLessThan(zebraIdx);

    const filterInput = screen.getByPlaceholderText(
      'Filter dimensions by key or value...',
    );
    fireEvent.change(filterInput, { target: { value: 'servohost' } });

    expect(screen.getByText('label-servo_hostname')).toBeVisible();
    expect(screen.queryByText('zebra-label')).not.toBeInTheDocument();
  });
});
