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
import { render, screen, fireEvent } from '@testing-library/react';

import { SettingsProvider } from '@/fleet/context/providers';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChromeOSDevice } from '../../device_list_page/chromeos/chromeos_fields';

import { ChromeOSDeviceDimensions } from './chromeos_device_dimensions';

const mockDevice = {
  id: 'device1',
  dutId: 'dut1',
  name: 'device1',
  state: 'READY',
  dutState: 'ready',
  realm: 'chromeos',
  deviceSpec: {
    labels: {
      'label-pool': { values: ['DUT_POOL_QUOTA'] },
      'label-board': { values: ['brya'] },
      'label-model': { values: ['gimble'] },
      servo_hostname: { values: ['servo-1'] },
      some_other_label: { values: ['val1'] },
      'label-phase': { values: ['EVT'] },
    },
  },
} as unknown as ChromeOSDevice;

describe('ChromeOSDeviceDimensions', () => {
  it('renders priority labels in correct order and allows filtering by key or value', () => {
    render(
      <FakeContextProvider>
        <SettingsProvider>
          <ChromeOSDeviceDimensions device={mockDevice} />
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const rows = screen.getAllByRole('row');
    const rowTexts = rows.map((r) => r.textContent);

    const servoIndex = rowTexts.findIndex((t) => t?.includes('Servo Hostname'));
    const poolIndex = rowTexts.findIndex((t) => t?.includes('Pool'));
    const otherIndex = rowTexts.findIndex((t) =>
      t?.includes('Some Other Label'),
    );

    expect(servoIndex).toBeGreaterThan(0);
    expect(servoIndex).toBeLessThan(poolIndex);
    expect(poolIndex).toBeLessThan(otherIndex);

    const searchInput = screen.getByPlaceholderText(
      'Filter dimensions by key or value...',
    );
    fireEvent.change(searchInput, { target: { value: 'servo_hostname' } });

    expect(screen.queryByText('Pool')).not.toBeInTheDocument();
    expect(screen.getByText('Servo Hostname')).toBeInTheDocument();

    fireEvent.change(searchInput, { target: { value: 'gimble' } });

    expect(screen.queryByText('Servo Hostname')).not.toBeInTheDocument();
    expect(screen.getByText('Model')).toBeInTheDocument();
    expect(screen.getByText('gimble')).toBeInTheDocument();

    fireEvent.change(searchInput, { target: { value: 'Some Other' } });
    expect(screen.getByText('Some Other Label')).toBeInTheDocument();
    expect(screen.queryByText('Model')).not.toBeInTheDocument();
  });
});
