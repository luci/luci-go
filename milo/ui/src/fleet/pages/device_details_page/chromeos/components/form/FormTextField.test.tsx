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
import { useState } from 'react';

import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

import { updateNestedValues } from '../../utils/inventory_editing_utils';

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
        label: 'Hive',
        path: 'chromeosMachineLse.deviceLse.dut.hive',
        editPath: 'hive',
        type: 'string',
      },
    ]),
  };
});

import { CardForm } from './CardForm';
import { FormTextField } from './FormTextField';
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

describe('<FormTextField />', () => {
  const initialLse = {
    chromeosMachineLse: {
      deviceLse: {
        dut: {
          pools: ['pool1', 'pool2'],
          hive: 'hive-1',
        },
      },
    },
  } as unknown as MachineLSE;

  it('renders read-only values when card is not in edit mode', () => {
    render(
      <TestWrapper initialLse={initialLse}>
        <FormTextField
          label="Pools"
          path="chromeosMachineLse.deviceLse.dut.pools"
          type="array"
        />
        <FormTextField
          label="Hive"
          path="chromeosMachineLse.deviceLse.dut.hive"
          type="string"
        />
      </TestWrapper>,
    );

    expect(screen.getByText('Pools')).toBeVisible();
    expect(screen.getByText('pool1')).toBeVisible();
    expect(screen.getByText('pool2')).toBeVisible();

    expect(screen.getByText('Hive')).toBeVisible();
    expect(screen.getByText('hive-1')).toBeVisible();
  });

  it('enters edit mode and allows updating string fields', async () => {
    const updateSpy = jest.fn();
    render(
      <TestWrapper initialLse={initialLse} onUpdateDraft={updateSpy}>
        <FormTextField
          label="Hive"
          path="chromeosMachineLse.deviceLse.dut.hive"
          type="string"
        />
      </TestWrapper>,
    );

    const editBtn = screen.getByRole('button', { name: /edit/i });
    await userEvent.click(editBtn);

    const input = screen.getByRole('textbox');
    expect(input).toHaveValue('hive-1');

    await userEvent.clear(input);
    await userEvent.type(input, 'new-hive');
    expect(input).toHaveValue('new-hive');

    const confirmBtn = screen.getByRole('button', { name: /confirm/i });
    await userEvent.click(confirmBtn);

    expect(updateSpy).toHaveBeenCalledWith([
      { path: 'chromeosMachineLse.deviceLse.dut.hive', value: 'new-hive' },
    ]);
  });

  it('handles array fields correctly with comma-separated inputs', async () => {
    const updateSpy = jest.fn();
    render(
      <TestWrapper initialLse={initialLse} onUpdateDraft={updateSpy}>
        <FormTextField
          label="Pools"
          path="chromeosMachineLse.deviceLse.dut.pools"
          type="array"
        />
      </TestWrapper>,
    );

    const editBtn = screen.getByRole('button', { name: /edit/i });
    await userEvent.click(editBtn);

    const input = screen.getByRole('textbox');
    expect(input).toHaveValue('pool1,pool2');

    await userEvent.clear(input);
    await userEvent.type(input, 'pool3, pool4');
    expect(input).toHaveValue('pool3, pool4');

    const confirmBtn = screen.getByRole('button', { name: /confirm/i });
    await userEvent.click(confirmBtn);

    expect(updateSpy).toHaveBeenCalledWith([
      {
        path: 'chromeosMachineLse.deviceLse.dut.pools',
        value: ['pool3', 'pool4'],
      },
    ]);
  });

  it('does not reset local input immediately when typing transient invalid number values', async () => {
    const localLse = {
      chromeosMachineLse: {
        deviceLse: {
          dut: {
            hive: '123',
          },
        },
      },
    } as unknown as MachineLSE;

    render(
      <TestWrapper initialLse={localLse}>
        <FormTextField
          label="Hive (Number)"
          path="chromeosMachineLse.deviceLse.dut.hive"
          type="number"
        />
      </TestWrapper>,
    );

    const editBtn = screen.getByRole('button', { name: /edit/i });
    await userEvent.click(editBtn);

    const input = screen.getByRole('spinbutton');
    expect(input).toHaveValue(123);

    // clear the number field - this will transiently trigger onChange with empty value
    await userEvent.clear(input);
    expect(input).toHaveValue(null);

    // type a negative sign
    await userEvent.type(input, '-');
    // In a number-type input, typing '-' may or may not display depending on the browser/jsdom implementation,
    // but the key point is it shouldn't crash or get forcefully reset.
    // Let's verify we can type a valid negative number:
    await userEvent.type(input, '45');
    expect(input).toHaveValue(-45);
  });
});
