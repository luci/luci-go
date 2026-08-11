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

import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';

import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

import {
  updateNestedValues,
  MAX_FIELD_LENGTH,
  MAX_ARRAY_TOTAL_LENGTH,
} from '../../utils/inventory_editing_utils';

jest.mock('../../utils/inventory_editing_utils', () => {
  const original = jest.requireActual('../../utils/inventory_editing_utils');
  return {
    ...original,
    getEditableFields: jest.fn(() => [
      {
        label: 'Pools',
        path: 'chromeosMachineLse.deviceLse.dut.pools',
        editPath: 'pools',
        type: 'array',
      },
      {
        label: 'RPM Type',
        path: 'chromeosMachineLse.deviceLse.dut.peripherals.rpm.type',
        editPath: 'rpm.type',
        type: 'string',
      },
    ]),
  };
});

import { CardForm } from './CardForm';
import { FormAutocompleteField } from './FormAutocompleteField';
import { InventoryFormProvider } from './InventoryFormContext';

interface TestWrapperProps {
  initialLse: MachineLSE;
  children: React.ReactNode;
  editable?: boolean;
  onUpdateDraft?: (
    updates: Array<{ path: string | string[]; value: unknown }>,
  ) => void;
}

const TestWrapper = ({
  initialLse,
  children,
  editable = true,
  onUpdateDraft,
}: TestWrapperProps) => {
  const [draftLse, setDraftLse] = useState<MachineLSE | null>(initialLse);
  const [activeCard, setActiveCard] = useState<string | null>(null);

  const handleUpdate = (
    updates: Array<{ path: string | string[]; value: unknown }>,
  ) => {
    onUpdateDraft?.(updates);
    setDraftLse((prev) => {
      if (!prev) return null;
      return updateNestedValues(
        prev as unknown as Record<string, unknown>,
        updates,
      ) as unknown as MachineLSE;
    });
  };

  return (
    <InventoryFormProvider
      originalLse={initialLse}
      draftLse={draftLse}
      updateDraftFields={handleUpdate}
      activeEditingCardId={activeCard}
      setActiveEditingCardId={setActiveCard}
      editable={editable}
    >
      <CardForm
        cardId="test-card"
        title="Test Card"
        isEmpty={false}
        emptyMessage=""
      >
        {children}
      </CardForm>
    </InventoryFormProvider>
  );
};

describe('<FormAutocompleteField />', () => {
  const initialLse = {
    chromeosMachineLse: {
      deviceLse: {
        dut: {
          pools: ['pool1', 'pool2'],
          peripherals: {
            rpm: {
              type: 'TYPE_APC',
            },
          },
        },
      },
    },
  } as unknown as MachineLSE;

  it('renders read-only values when card is not in edit mode', () => {
    render(
      <TestWrapper initialLse={initialLse}>
        <FormAutocompleteField
          label="Pools"
          path="chromeosMachineLse.deviceLse.dut.pools"
          multiple
        />
      </TestWrapper>,
    );

    expect(screen.queryByRole('button', { name: /edit/i })).toBeInTheDocument();
    expect(screen.getByText('pool1')).toBeInTheDocument();
    expect(screen.getByText('pool2')).toBeInTheDocument();
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
  });

  it('enters edit mode and allows selecting options (multiple)', async () => {
    const onUpdateDraft = jest.fn();
    render(
      <TestWrapper initialLse={initialLse} onUpdateDraft={onUpdateDraft}>
        <FormAutocompleteField
          label="Pools"
          path="chromeosMachineLse.deviceLse.dut.pools"
          options={['pool1', 'pool2', 'pool3']}
          multiple
        />
      </TestWrapper>,
    );

    const editBtn = screen.getByRole('button', { name: /edit/i });
    await userEvent.click(editBtn);

    const input = screen.getByRole('combobox', { name: /pools/i });
    await userEvent.click(input);

    const option = await screen.findByRole('option', { name: 'pool3' });
    fireEvent.click(option);

    const confirmBtn = screen.getByRole('button', { name: /confirm/i });
    await userEvent.click(confirmBtn);

    expect(onUpdateDraft).toHaveBeenLastCalledWith([
      {
        path: 'chromeosMachineLse.deviceLse.dut.pools',
        value: ['pool1', 'pool2', 'pool3'],
      },
    ]);
  });

  it('validates total combined length for multiple selections', async () => {
    const onUpdateDraft = jest.fn();
    const option1 = 'a'.repeat(80);
    const option2 = 'b'.repeat(80);
    const option3 = 'c'.repeat(80);
    render(
      <TestWrapper initialLse={initialLse} onUpdateDraft={onUpdateDraft}>
        <FormAutocompleteField
          label="Pools"
          path="chromeosMachineLse.deviceLse.dut.pools"
          options={['pool1', 'pool2', option1, option2, option3]}
          multiple
        />
      </TestWrapper>,
    );

    const editBtn = screen.getByRole('button', { name: /edit/i });
    await userEvent.click(editBtn);

    const input = screen.getByRole('combobox', { name: /pools/i });
    await userEvent.click(input);

    // Select option1
    let option = await screen.findByRole('option', { name: option1 });
    fireEvent.click(option);

    // Select option2
    await userEvent.click(input);
    option = await screen.findByRole('option', { name: option2 });
    fireEvent.click(option);

    // Select option3 (this should push total past 200)
    await userEvent.click(input);
    option = await screen.findByRole('option', { name: option3 });
    fireEvent.click(option);

    expect(
      screen.getByText(
        `Total combined length exceeds ${MAX_ARRAY_TOTAL_LENGTH} characters`,
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/Maximum length for each item is/),
    ).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /confirm/i })).toBeDisabled();

    expect(onUpdateDraft).not.toHaveBeenCalled();
  });

  it('validates individual item length in multiple mode', async () => {
    const longCustomOption = 'a'.repeat(MAX_FIELD_LENGTH + 10);
    render(
      <TestWrapper initialLse={initialLse}>
        <FormAutocompleteField
          label="Pools"
          path="chromeosMachineLse.deviceLse.dut.pools"
          multiple
          freeSolo
        />
      </TestWrapper>,
    );

    const editBtn = screen.getByRole('button', { name: /edit/i });
    await userEvent.click(editBtn);

    const input = screen.getByRole('combobox', { name: /pools/i });
    await userEvent.type(input, `${longCustomOption}{enter}`);

    expect(
      screen.getByText(
        `Maximum length for each item is ${MAX_FIELD_LENGTH} characters (${longCustomOption})`,
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /confirm/i })).toBeDisabled();
  });

  it('validates item length in single mode (multiple=false)', async () => {
    const longCustomOption = 'a'.repeat(MAX_FIELD_LENGTH + 10);
    render(
      <TestWrapper initialLse={initialLse}>
        <FormAutocompleteField
          label="RPM Type"
          path="chromeosMachineLse.deviceLse.dut.peripherals.rpm.type"
          multiple={false}
          freeSolo
        />
      </TestWrapper>,
    );

    const editBtn = screen.getByRole('button', { name: /edit/i });
    await userEvent.click(editBtn);

    const input = screen.getByRole('combobox', { name: /rpm type/i });
    await userEvent.clear(input);
    await userEvent.type(input, `${longCustomOption}{enter}`);

    expect(
      screen.getByText(`Maximum length is ${MAX_FIELD_LENGTH} characters`),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /confirm/i })).toBeDisabled();
  });
});
