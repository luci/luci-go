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

import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { InventoryFormProvider } from '../form/InventoryFormContext';

import { ServoHardwareCard } from './ServoHardwareCard';

const mockLseWithServo = (
  servo?: {
    servoHostname?: string | null;
    servoSerial?: string | null;
    servoPort?: number | null;
  } | null,
  isLabstation = false,
) => {
  const peripherals = servo ? { servo } : undefined;
  const deviceLse = isLabstation
    ? { labstation: {} }
    : { dut: { peripherals } };

  return {
    chromeosMachineLse: {
      deviceLse,
    },
  };
};

interface RenderOptions {
  servo?: {
    servoHostname?: string | null;
    servoSerial?: string | null;
    servoPort?: number | null;
  } | null;
  editable?: boolean;
  activeEditingCardId?: string | null;
  isLabstation?: boolean;
}

const renderCard = (options: RenderOptions = {}) => {
  const updateDraftFields = jest.fn();
  const setActiveEditingCardId = jest.fn();
  const lse = mockLseWithServo(options.servo, options.isLabstation);

  return {
    updateDraftFields,
    setActiveEditingCardId,
    ...render(
      <FakeContextProvider>
        <InventoryFormProvider
          originalLse={lse as unknown as MachineLSE}
          draftLse={lse as unknown as MachineLSE}
          updateDraftFields={updateDraftFields}
          activeEditingCardId={options.activeEditingCardId ?? null}
          setActiveEditingCardId={setActiveEditingCardId}
          editable={options.editable || false}
        >
          <ServoHardwareCard />
        </InventoryFormProvider>
      </FakeContextProvider>,
    ),
  };
};

describe('<ServoHardwareCard />', () => {
  it('renders servo hardware telemetry correctly', async () => {
    renderCard({
      servo: {
        servoHostname: 'chromeos15-row1-labstation1',
        servoSerial: 'SERVO-SN-9988',
        servoPort: 9999,
      },
    });

    expect(screen.getByText('Servo')).toBeVisible();
    expect(screen.getByText('Hostname')).toBeVisible();
    expect(screen.getByText('Serial')).toBeVisible();
    expect(screen.getByText('Port')).toBeVisible();
    expect(screen.getByText('chromeos15-row1-labstation1')).toBeVisible();
    expect(screen.getByText('SERVO-SN-9988')).toBeVisible();
    expect(screen.getByText('9999')).toBeVisible();
  });

  it('hides omitted fields and does not show N/A fallbacks', async () => {
    renderCard({
      servo: {
        servoHostname: 'labstation-host-2',
      },
    });

    expect(screen.getByText('Servo')).toBeVisible();
    expect(screen.getByText('Hostname')).toBeVisible();
    expect(screen.getByText('labstation-host-2')).toBeVisible();

    expect(screen.queryByText('Serial')).toBeNull();
    expect(screen.queryByText('Port')).toBeNull();
    expect(screen.queryByText('N/A')).toBeNull();
  });

  it('renders empty message when no servo telemetry exists', async () => {
    renderCard({});
    expect(
      screen.getByText('No Servo debugging hardware attached.'),
    ).toBeVisible();
  });

  it('renders edit button and handles onEdit click when editable is true on a DUT', async () => {
    const { setActiveEditingCardId } = renderCard({
      servo: {
        servoHostname: 'chromeos15-row1-labstation1',
      },
      editable: true,
    });

    const editBtn = screen.getByRole('button', {
      name: 'edit Servo',
    });
    expect(editBtn).toBeVisible();
    await userEvent.click(editBtn);
    expect(setActiveEditingCardId).toHaveBeenCalledWith('servo');
  });

  it('renders input fields for Hostname, Serial, and Port when in edit mode', async () => {
    renderCard({
      servo: {
        servoHostname: 'chromeos15-row1-labstation1',
        servoSerial: 'SERVO-SN-1234',
        servoPort: 9999,
      },
      editable: true,
      activeEditingCardId: 'servo',
    });

    expect(screen.getByLabelText('Hostname')).toHaveValue(
      'chromeos15-row1-labstation1',
    );
    expect(screen.getByLabelText('Serial')).toHaveValue('SERVO-SN-1234');
    expect(screen.getByLabelText('Port')).toHaveValue(9999);
  });

  it('enforces Servo Port range limits (9000-9999) automatically from FieldConfig', async () => {
    renderCard({
      servo: {
        servoPort: 9999,
      },
      editable: true,
      activeEditingCardId: 'servo',
    });

    const portInput = screen.getByLabelText('Port');
    await userEvent.clear(portInput);
    await userEvent.type(portInput, '8888');

    expect(
      screen.getByText('Must be 0 or between 9000 and 9999'),
    ).toBeInTheDocument();
  });
});
