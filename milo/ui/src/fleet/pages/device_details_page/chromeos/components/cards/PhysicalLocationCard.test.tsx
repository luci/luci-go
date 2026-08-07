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

import { PhysicalLocationCard } from './PhysicalLocationCard';

const renderWithProviders = (ui: React.ReactNode) =>
  render(<FakeContextProvider>{ui}</FakeContextProvider>);

describe('<PhysicalLocationCard />', () => {
  it('renders location telemetry correctly', async () => {
    renderWithProviders(
      <PhysicalLocationCard
        zone="ZONE_CHROMEOS15"
        rack="chromeos15-row1-metro1"
      />,
    );

    expect(
      screen.getByText('Physical Location & Infrastructure'),
    ).toBeVisible();
    expect(screen.getByText('Zone')).toBeVisible();
    expect(screen.getByText('Rack')).toBeVisible();

    expect(screen.getByText('ZONE_CHROMEOS15')).toBeVisible();
    expect(screen.getByText('chromeos15-row1-metro1')).toBeVisible();
  });

  it('renders empty message when no location telemetry exists', () => {
    renderWithProviders(<PhysicalLocationCard />);
    expect(
      screen.getByText('No physical location metadata recorded.'),
    ).toBeInTheDocument();
  });

  it('hides omitted fields when partial location data exists', () => {
    renderWithProviders(<PhysicalLocationCard zone="ZONE_CHROMEOS1" />);
    expect(screen.getByText('ZONE_CHROMEOS1')).toBeInTheDocument();
    expect(screen.queryByText('Rack')).not.toBeInTheDocument();
    expect(screen.queryByText('N/A')).not.toBeInTheDocument();
  });

  it('renders edit button and handles onEdit click when editable is true', async () => {
    const handleEdit = jest.fn();
    renderWithProviders(
      <PhysicalLocationCard
        zone="ZONE_CHROMEOS15"
        editable
        onEdit={handleEdit}
      />,
    );

    const editBtn = screen.getByRole('button', {
      name: 'edit Physical Location & Infrastructure',
    });
    expect(editBtn).toBeVisible();
    await userEvent.click(editBtn);
    expect(handleEdit).toHaveBeenCalledTimes(1);
  });

  it('renders FormAutocompleteField for Zone and FormTextField for Rack when editing is enabled and both are editable', async () => {
    const updateDraftFields = jest.fn();
    const setActiveEditingCardId = jest.fn();
    const lse = {
      name: 'machineLSEs/test-host',
      zone: 'ZONE_CHROMEOS1',
      rack: 'rack-1',
    } as unknown as MachineLSE;

    render(
      <FakeContextProvider>
        <InventoryFormProvider
          originalLse={lse}
          draftLse={lse}
          updateDraftFields={updateDraftFields}
          activeEditingCardId="location"
          setActiveEditingCardId={setActiveEditingCardId}
          editable={true}
        >
          <PhysicalLocationCard
            zone="ZONE_CHROMEOS1"
            rack="rack-1"
            editable={true}
          />
        </InventoryFormProvider>
      </FakeContextProvider>,
    );

    const combobox = screen.getByRole('combobox', { name: /zone/i });
    expect(combobox).toBeInTheDocument();
    expect(combobox).toHaveValue('ZONE_CHROMEOS1');

    const rackInput = screen.getByRole('textbox', { name: /rack/i });
    expect(rackInput).toBeInTheDocument();
    await userEvent.clear(rackInput);
    await userEvent.type(rackInput, 'rack-2');
    expect(rackInput).toHaveValue('rack-2');

    const confirmBtn = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmBtn).toBeVisible();
    await userEvent.click(confirmBtn);

    expect(updateDraftFields).toHaveBeenCalledWith([
      { path: 'rack', value: 'rack-2' },
    ]);
  });
});
