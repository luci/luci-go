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

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { generateServoHostnameFilterURL } from '@/fleet/config/chromeos_device_config';
import { SettingsProvider } from '@/fleet/context/providers';
import {
  ChromeOSDevice,
  getFieldDefinition,
} from '@/fleet/pages/device_list_page/chromeos/chromeos_fields';
import { FC_CellProps } from '@/fleet/types/table';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSDeviceDetailsPage } from './chromeos_device_details_page';
import { useChromeOSDeviceData } from './use_chromeos_device_data';

jest.mock('./use_chromeos_device_data');

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
});
