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

import '@testing-library/jest-dom';
import { render, screen } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AndroidDeviceDetailsPage } from './android_device_details_page';
import * as deviceDataModule from './use_android_device_data';

jest.mock('./use_android_device_data');

const mockDevice = AndroidDevice.fromPartial({
  id: 'android-device-42',
  realm: 'android:test',
  omnilabSpec: {
    labels: {
      battery_level: { values: ['92'] },
      host_group: { values: ['lab_hosts'] },
      'ufs.last_sync': { values: ['2026-03-01T12:00:00Z'] },
      location_tag: { values: ['shelf-3'] },
    },
  },
});

describe('AndroidDeviceDetailsPage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.spyOn(deviceDataModule, 'useAndroidDeviceData').mockReturnValue({
      isError: false,
      isLoading: false,
      device: mockDevice,
    });
  });

  it('renders centralized humanized labels in alphabetical order', () => {
    render(
      <FakeContextProvider
        mountedPath="/ui/fleet/p/:platform/devices/:id"
        routerOptions={{
          initialEntries: ['/ui/fleet/p/android/devices/android-device-42'],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <AndroidDeviceDetailsPage workspace="Android" />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    // Verify humanized labels are displayed instead of raw keys
    expect(screen.getByText('Battery Level')).toBeInTheDocument();
    expect(screen.getByText('Host Group')).toBeInTheDocument();
    expect(screen.getByText('UFS Last Sync')).toBeInTheDocument();
    expect(screen.getByText('location_tag')).toBeInTheDocument();
    expect(screen.getByText('Realm')).toBeInTheDocument();

    // Verify raw keys without override are not rendered as labels
    expect(screen.queryByText('battery_level')).not.toBeInTheDocument();
    expect(screen.queryByText('host_group')).not.toBeInTheDocument();

    // Verify row ordering is based on humanized labels
    const rows = screen.getAllByRole('row');
    const rowTexts = rows.map((r) => r.textContent);

    const batteryIdx = rowTexts.findIndex((t) => t?.includes('Battery Level'));
    const hostGroupIdx = rowTexts.findIndex((t) => t?.includes('Host Group'));
    const realmIdx = rowTexts.findIndex((t) => t?.includes('Realm'));
    const ufsIdx = rowTexts.findIndex((t) => t?.includes('UFS Last Sync'));

    expect(batteryIdx).toBeGreaterThan(0);
    expect(batteryIdx).toBeLessThan(hostGroupIdx);
    expect(hostGroupIdx).toBeLessThan(realmIdx);
    expect(realmIdx).toBeLessThan(ufsIdx);
  });
});
