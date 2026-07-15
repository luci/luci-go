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

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ServoHardwareCard } from './ServoHardwareCard';

describe('<ServoHardwareCard />', () => {
  it('renders servo hardware telemetry correctly', async () => {
    render(
      <FakeContextProvider>
        <ServoHardwareCard
          servo={{
            servoHostname: 'chromeos15-row1-labstation1',
            servoSerial: 'SERVO-SN-9988',
            servoPort: 9999,
          }}
        />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Servo')).toBeVisible();
    expect(screen.getByText('Hostname')).toBeVisible();
    expect(screen.getByText('Serial')).toBeVisible();
    expect(screen.getByText('Port')).toBeVisible();
    expect(screen.getByText('chromeos15-row1-labstation1')).toBeVisible();
    expect(screen.getByText('SERVO-SN-9988')).toBeVisible();
    expect(screen.getByText('9999')).toBeVisible();
  });

  it('hides omitted fields and does not show N/A fallbacks', async () => {
    render(
      <FakeContextProvider>
        <ServoHardwareCard
          servo={{
            servoHostname: 'labstation-host-2',
          }}
        />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Servo')).toBeVisible();
    expect(screen.getByText('Hostname')).toBeVisible();
    expect(screen.getByText('labstation-host-2')).toBeVisible();

    expect(screen.queryByText('Serial')).toBeNull();
    expect(screen.queryByText('Port')).toBeNull();
    expect(screen.queryByText('N/A')).toBeNull();
  });

  it('renders empty message when no servo telemetry exists', async () => {
    render(
      <FakeContextProvider>
        <ServoHardwareCard />
      </FakeContextProvider>,
    );
    expect(
      screen.getByText('No Servo debugging hardware attached.'),
    ).toBeVisible();
  });

  it('renders edit button and handles onEdit click when editable is true', async () => {
    const handleEdit = jest.fn();
    render(
      <FakeContextProvider>
        <ServoHardwareCard
          servo={{
            servoHostname: 'chromeos15-row1-labstation1',
          }}
          editable
          onEdit={handleEdit}
        />
      </FakeContextProvider>,
    );

    const editBtn = screen.getByRole('button', {
      name: 'edit Servo',
    });
    expect(editBtn).toBeVisible();
    await userEvent.click(editBtn);
    expect(handleEdit).toHaveBeenCalledTimes(1);
  });
});
