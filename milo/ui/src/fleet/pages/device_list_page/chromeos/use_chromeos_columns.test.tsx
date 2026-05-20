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

import { renderHook } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';

import { getFieldDefinition } from './chromeos_fields';
import { useChromeOSFields } from './use_chromeos_available_columns';
import { useChromeOSColumns } from './use_chromeos_columns';

jest.mock('./use_chromeos_available_columns', () => ({
  useChromeOSFields: jest.fn(),
}));

jest.mock('./chromeos_fields', () => {
  const actual = jest.requireActual('./chromeos_fields');
  return {
    ...actual,
    getFieldDefinition: jest
      .fn()
      .mockImplementation((id) => actual.getFieldDefinition(id)),
  };
});

describe('useChromeOSColumns', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('should include missing columns from URL in availableColumns', () => {
    (useChromeOSFields as jest.Mock).mockReturnValue({
      availableFields: [
        { id: 'id', columnDef: { id: 'id', header: 'ID' } },
        {
          id: 'label-board',
          columnDef: { id: 'label-board', header: 'Board' },
        },
        { id: 'dut_id', columnDef: { id: 'dut_id', header: 'Dut ID' } },
        {
          id: 'current_task',
          columnDef: { id: 'current_task', header: 'Current Task' },
        },
        { id: 'realm', columnDef: { id: 'realm', header: 'Realm' } },
      ],
      isLoading: false,
    });

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/?c=id&c=dut_name']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useChromeOSColumns(false, false), {
      wrapper,
    });

    const availableColumnIds = result.current.availableColumns.map((c) => c.id);

    // Should include columns from availableFields
    expect(availableColumnIds).toContain('id');
    expect(availableColumnIds).toContain('label-board');

    // Should also include missing column from URL
    expect(availableColumnIds).toContain('dut_name');

    // Should NOT generate warnings for missing but valid columns
    expect(result.current.warnings).toEqual([]);
  });
  it('should handle the specific bug repro case with missing columns', () => {
    (useChromeOSFields as jest.Mock).mockReturnValue({
      availableFields: [
        { id: 'id', columnDef: { id: 'id', header: 'ID' } },
        {
          id: 'label-board',
          columnDef: { id: 'label-board', header: 'Board' },
        },
        {
          id: 'label-model',
          columnDef: { id: 'label-model', header: 'Model' },
        },
        { id: 'dut_id', columnDef: { id: 'dut_id', header: 'Dut ID' } },
        {
          id: 'current_task',
          columnDef: { id: 'current_task', header: 'Current Task' },
        },
        { id: 'realm', columnDef: { id: 'realm', header: 'Realm' } },
      ],
      isLoading: false,
    });

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={[
          '/?c=id&c=dut_name&c=label-board&c=label-carrier&c=label-cellular_variant&c=label-model&c=label-modem_type',
        ]}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useChromeOSColumns(false, false), {
      wrapper,
    });

    const availableColumnIds = result.current.availableColumns.map((c) => c.id);

    expect(availableColumnIds).toContain('id');
    expect(availableColumnIds).toContain('label-board');
    expect(availableColumnIds).toContain('label-model');
    expect(availableColumnIds).toContain('dut_name');
    expect(availableColumnIds).toContain('label-carrier');
    expect(availableColumnIds).toContain('label-cellular_variant');
    expect(availableColumnIds).toContain('label-modem_type');

    expect(result.current.warnings).toEqual([]);
  });
  it('should handle invalid column IDs in URL by filtering out null columnDefs', () => {
    (useChromeOSFields as jest.Mock).mockReturnValue({
      availableFields: [{ id: 'id', columnDef: { id: 'id', header: 'ID' } }],
      isLoading: false,
    });

    // Mock getFieldDefinition to return null for a specific invalid ID
    (getFieldDefinition as jest.Mock).mockImplementation((id) => {
      if (id === 'invalid-col') {
        return null;
      }
      return jest.requireActual('./chromeos_fields').getFieldDefinition(id);
    });

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/?c=id&c=invalid-col']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useChromeOSColumns(false, false), {
      wrapper,
    });

    const availableColumnIds = result.current.availableColumns.map((c) => c.id);

    // Should include 'id'
    expect(availableColumnIds).toContain('id');

    // Should NOT include 'invalid-col' because it was filtered out!
    expect(availableColumnIds).not.toContain('invalid-col');
  });
  it('should preserve the order of availableColumns in visibleColumns', () => {
    (useChromeOSFields as jest.Mock).mockReturnValue({
      availableFields: [
        { id: 'id', columnDef: { id: 'id', header: 'ID' } },
        {
          id: 'label-board',
          columnDef: { id: 'label-board', header: 'Board' },
        },
        { id: 'realm', columnDef: { id: 'realm', header: 'Realm' } },
      ],
      isLoading: false,
    });

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/?c=realm&c=id']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useChromeOSColumns(false, false), {
      wrapper,
    });

    const visibleColumnIds = result.current.visibleColumns.map((c) => c.id);

    // The URL requested 'realm' then 'id'.
    // But availableColumns has 'id' then 'realm' (since it follows availableFields order).
    // So visibleColumns should have 'id' then 'realm'!
    expect(visibleColumnIds).toEqual(['id', 'realm']);
  });

  it('should fall back to default columns if URL has no columns', () => {
    (useChromeOSFields as jest.Mock).mockReturnValue({
      availableFields: [
        { id: 'id', columnDef: { id: 'id', header: 'ID' } },
        {
          id: 'label-pool',
          columnDef: { id: 'label-pool', header: 'label-pool' },
        },
      ],
      isLoading: false,
    });

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useChromeOSColumns(false, false), {
      wrapper,
    });

    const availableColumnIds = result.current.availableColumns.map((c) => c.id);

    // Should include columns from availableFields
    expect(availableColumnIds).toContain('id');

    // Should also include default columns (which are calculated in useChromeOSColumns)
    // and merged into visibleColumnIds, and then added to availableColumns if missing.
    expect(availableColumnIds).toContain('label-pool');
  });
});
