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

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { State } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/state.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { DeviceDetailsCard } from './DeviceDetailsCard';

describe('<DeviceDetailsCard />', () => {
  it('renders host telemetry correctly', async () => {
    render(
      <FakeContextProvider>
        <DeviceDetailsCard
          data={{
            hostname: 'chromeos15-row1-host1',
            name: 'machineLSEs/chromeos15-row1-host1',
            realm: '@internal:ufs/os-acs',
            machines: ['ASSET-1234'],
          }}
        />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Device Details')).toBeVisible();
    expect(screen.getByText('chromeos15-row1-host1')).toBeVisible();
    expect(screen.getByText('machineLSEs/chromeos15-row1-host1')).toBeVisible();
    expect(screen.getByText('@internal:ufs/os-acs')).toBeVisible();
    expect(screen.getByText('ASSET-1234')).toBeVisible();
  });

  it('renders optional fields and formatted resourceState/timestamps correctly', async () => {
    render(
      <FakeContextProvider>
        <DeviceDetailsCard
          data={{
            hostname: 'chromeos15-row1-host1',
            resourceState: State.STATE_SERVING,
            description: 'Primary testbed labstation',
            deploymentTicket: 'b/1234567',
            machineLsePrototype: 'machineLSEPrototypes/labstation',
            updateTime: '2026-06-01T12:00:00Z',
          }}
        />
      </FakeContextProvider>,
    );

    expect(screen.getByText('SERVING')).toBeVisible();
    expect(screen.getByText('Primary testbed labstation')).toBeVisible();
    expect(screen.getByText('b/1234567')).toBeVisible();
    expect(screen.getByText('machineLSEPrototypes/labstation')).toBeVisible();
    expect(screen.getByText('Last Modified in UFS')).toBeVisible();
  });

  it('renders edit button and handles onEdit click when editable is true', async () => {
    const handleEdit = jest.fn();
    render(
      <FakeContextProvider>
        <DeviceDetailsCard
          data={{
            hostname: 'chromeos15-row1-host1',
          }}
          editable
          onEdit={handleEdit}
        />
      </FakeContextProvider>,
    );

    const editBtn = screen.getByRole('button', {
      name: 'edit Device Details',
    });
    expect(editBtn).toBeVisible();
    await userEvent.click(editBtn);
    expect(handleEdit).toHaveBeenCalledTimes(1);
  });
});
