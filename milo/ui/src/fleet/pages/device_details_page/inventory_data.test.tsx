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

import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { InventoryData } from './inventory_data';

describe('<InventoryData />', () => {
  const mockDeviceOs = {
    id: 'test-host-name',
    deviceSpec: {
      labels: {},
    },
  } as Partial<Device> as Device;

  const mockDeviceOsPartner = {
    id: 'test-host-name-partner',
    deviceSpec: {
      labels: {
        ufs_namespace: {
          values: ['os-partner'],
        },
      },
    },
  } as Partial<Device> as Device;

  it('renders CLI command for default namespace', async () => {
    render(
      <FakeContextProvider>
        <InventoryData device={mockDeviceOs} />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('$ shivas get dut -json test-host-name'),
    ).toBeVisible();
  });

  it('renders CLI command for os-partner namespace', async () => {
    render(
      <FakeContextProvider>
        <InventoryData device={mockDeviceOsPartner} />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText(
        '$ shivas get dut -json -namespace os-partner test-host-name-partner',
      ),
    ).toBeVisible();
  });
});
