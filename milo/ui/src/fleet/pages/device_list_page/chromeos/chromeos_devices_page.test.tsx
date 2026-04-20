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

import { render, screen, fireEvent } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSDevicesPage } from './chromeos_devices_page';

jest.mock('@/fleet/hooks/use_devices', () => ({
  useDevices: () => ({
    data: {
      devices: [
        { id: '1', dutId: 'dut-1' },
        { id: '2', dutId: 'dut-2' },
      ],
    },
    isPending: false,
    isFetching: false,
  }),
}));

describe('<ChromeOSDevicesPage />', () => {
  it('should render', async () => {
    render(
      <FakeContextProvider
        mountedPath="/test/:platform"
        routerOptions={{
          initialEntries: ['/test/chromeos'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <ChromeOSDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('Main metrics')).toBeVisible();
  });

  it('should allow clicking action buttons when rows are selected', async () => {
    render(
      <FakeContextProvider
        mountedPath="/test/:platform"
        routerOptions={{
          initialEntries: ['/test/chromeos'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <ChromeOSDevicesPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Wait for data to load and rows to appear
    const checkboxes = await screen.findAllByRole('checkbox');
    expect(checkboxes.length).toBeGreaterThan(1);

    // Click the first row checkbox (index 1, index 0 is select all)
    fireEvent.click(checkboxes[1]);

    // Check that "Run autorepair" button is visible
    const autorepairButton = screen.getByRole('button', {
      name: /run autorepair/i,
    });
    expect(autorepairButton).toBeVisible();

    // Click it
    fireEvent.click(autorepairButton);
  });
});
