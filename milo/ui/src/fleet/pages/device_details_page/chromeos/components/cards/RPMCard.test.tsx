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

import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { InventoryFormProvider } from '../form/InventoryFormContext';

import { RPMCard } from './RPMCard';

const mockLseWithRpm = (
  rpm?: {
    powerunitName?: string | null;
    powerunitOutlet?: string | null;
    powerunitType?: number | null;
  } | null,
  isLabstation = false,
) => {
  const deviceLse = isLabstation
    ? { labstation: { rpm } }
    : { dut: { peripherals: { rpm } } };

  return {
    chromeosMachineLse: {
      deviceLse,
    },
  };
};

interface RenderOptions {
  rpm?: {
    powerunitName?: string | null;
    powerunitOutlet?: string | null;
    powerunitType?: number | null;
  } | null;
  editable?: boolean;
  activeEditingCardId?: string | null;
  isLabstation?: boolean;
}

const renderCard = (options: RenderOptions = {}) => {
  const updateDraftFields = jest.fn();
  const setActiveEditingCardId = jest.fn();
  const lse = mockLseWithRpm(options.rpm, options.isLabstation);

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
          <RPMCard />
        </InventoryFormProvider>
      </FakeContextProvider>,
    ),
  };
};

describe('<RPMCard />', () => {
  it('renders DUT power RPM connection correctly', async () => {
    renderCard({
      rpm: {
        powerunitName: 'chromeos15-row1-rpm',
        powerunitOutlet: 'AA1',
        powerunitType: 1, // TYPE_SENTRY
      },
    });

    expect(screen.getByText('RPM')).toBeVisible();
    expect(screen.getByText('Name')).toBeVisible();
    expect(screen.getByText('Outlet')).toBeVisible();
    expect(screen.getByText('Type')).toBeVisible();
    expect(screen.getByText('chromeos15-row1-rpm')).toBeVisible();
    expect(screen.getByText('AA1')).toBeVisible();
    expect(screen.getByText('SENTRY')).toBeVisible();
  });

  it('hides empty fields when partial details exist', async () => {
    renderCard({
      rpm: {
        powerunitName: 'chromeos15-row1-rpm',
      },
    });

    expect(screen.getByText('RPM')).toBeVisible();
    expect(screen.getByText('Name')).toBeVisible();
    expect(screen.getByText('chromeos15-row1-rpm')).toBeVisible();

    expect(screen.queryByText('Outlet')).toBeNull();
    expect(screen.queryByText('Type')).toBeNull();
    expect(screen.queryByText('N/A')).toBeNull();
  });

  it('renders empty message when no RPM telemetry exists', () => {
    renderCard({});
    expect(
      screen.getByText(
        'No RPM outlets or power distribution units configured.',
      ),
    ).toBeVisible();
  });

  it('renders edit button and handles onEdit click when editable is true on a DUT', async () => {
    const { setActiveEditingCardId } = renderCard({
      rpm: {
        powerunitName: 'chromeos15-row1-rpm',
      },
      editable: true,
    });

    const editBtn = screen.getByRole('button', {
      name: 'edit RPM',
    });
    expect(editBtn).toBeVisible();
    await userEvent.click(editBtn);
    expect(setActiveEditingCardId).toHaveBeenCalledWith('rpm');
  });

  it('renders input fields for Name, Outlet, and Type when in edit mode', async () => {
    renderCard({
      rpm: {
        powerunitName: 'chromeos15-row1-rpm',
        powerunitOutlet: 'AA1',
        powerunitType: 1, // SENTRY
      },
      editable: true,
      activeEditingCardId: 'rpm',
    });

    expect(screen.getByLabelText('Name')).toHaveValue('chromeos15-row1-rpm');
    expect(screen.getByLabelText('Outlet')).toHaveValue('AA1');
    expect(screen.getByLabelText('Type')).toHaveValue('SENTRY');
  });

  it('allows selecting RPM Type from autocomplete options', async () => {
    const { updateDraftFields } = renderCard({
      rpm: {
        powerunitName: 'chromeos15-row1-rpm',
        powerunitOutlet: 'AA1',
        powerunitType: 1, // SENTRY
      },
      editable: true,
      activeEditingCardId: 'rpm',
    });

    const typeInput = screen.getByLabelText('Type');
    await userEvent.click(typeInput);

    // Find the option in listbox
    const listbox = screen.getByRole('listbox');
    const option = within(listbox).getByText('IP9850');
    await userEvent.click(option);

    expect(typeInput).toHaveValue('IP9850');

    // Confirm the form to trigger updateDraftFields
    const confirmBtn = screen.getByRole('button', { name: 'Confirm' });
    await userEvent.click(confirmBtn);

    // Value mapping should have stored it as number (TYPE_IP9850 = 2)
    // Verify that updateDraftFields is called with the correct path and mapped enum value.
    expect(updateDraftFields).toHaveBeenCalledWith([
      {
        path: 'chromeosMachineLse.deviceLse.dut.peripherals.rpm.powerunitType',
        value: 2, // OSRPM_Type.TYPE_IP9850
      },
    ]);
  });
});
