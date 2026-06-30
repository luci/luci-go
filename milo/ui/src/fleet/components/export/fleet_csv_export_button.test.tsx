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

import { FleetCSVExportButton } from './fleet_csv_export_button';

describe('FleetCSVExportButton', () => {
  it('should render export button and execute export', async () => {
    const mockExport = jest
      .fn()
      .mockResolvedValue({ csvData: 'id,name\n1,foo' });
    const fakeTable = {
      getVisibleLeafColumns: () => [{ id: 'id', columnDef: { header: 'ID' } }],
      getRowModel: () => ({ rows: [{ id: 'dut-1' }] }),
      getState: () => ({ rowSelection: {} }),
    } as unknown as Parameters<typeof FleetCSVExportButton>[0]['table'];

    render(
      <FakeContextProvider>
        <FleetCSVExportButton
          table={fakeTable}
          filter="state:ready"
          fileName="test_export"
          onExport={mockExport}
        />
      </FakeContextProvider>,
    );

    const exportBtn = screen.getByRole('button', { name: /export/i });
    expect(exportBtn).toBeVisible();
    await userEvent.click(exportBtn);

    const allBtn = await screen.findByText('Export all (CSV)');
    await userEvent.click(allBtn);
    expect(mockExport).toHaveBeenCalledWith(
      [{ name: 'id', displayName: 'ID' }],
      'state:ready',
      undefined,
    );
  });

  it('should render export button and execute export for current page', async () => {
    const mockExport = jest
      .fn()
      .mockResolvedValue({ csvData: 'id,name\n1,foo' });
    const fakeTable = {
      getVisibleLeafColumns: () => [{ id: 'id', columnDef: { header: 'ID' } }],
      getRowModel: () => ({ rows: [{ id: 'dut-1' }] }),
      getState: () => ({ rowSelection: {} }),
    } as unknown as Parameters<typeof FleetCSVExportButton>[0]['table'];

    render(
      <FakeContextProvider>
        <FleetCSVExportButton
          table={fakeTable}
          filter="state:ready"
          fileName="test_export"
          onExport={mockExport}
        />
      </FakeContextProvider>,
    );

    const exportBtn = screen.getByRole('button', { name: /export/i });
    await userEvent.click(exportBtn);

    const pageBtn = await screen.findByText('Export current page (CSV)');
    await userEvent.click(pageBtn);
    expect(mockExport).toHaveBeenCalledWith(
      [{ name: 'id', displayName: 'ID' }],
      'state:ready',
      ['dut-1'],
    );
  });

  it('should render export button and execute export for selected rows', async () => {
    const mockExport = jest
      .fn()
      .mockResolvedValue({ csvData: 'id,name\n1,foo' });
    const fakeTable = {
      getVisibleLeafColumns: () => [{ id: 'id', columnDef: { header: 'ID' } }],
      getRowModel: () => ({ rows: [{ id: 'dut-1' }] }),
      getState: () => ({ rowSelection: { 'dut-1': true } }),
    } as unknown as Parameters<typeof FleetCSVExportButton>[0]['table'];

    render(
      <FakeContextProvider>
        <FleetCSVExportButton
          table={fakeTable}
          filter="state:ready"
          fileName="test_export"
          onExport={mockExport}
        />
      </FakeContextProvider>,
    );

    const exportBtn = screen.getByRole('button', { name: /export/i });
    await userEvent.click(exportBtn);

    const selectedBtn = await screen.findByText('Export selected (CSV)');
    await userEvent.click(selectedBtn);
    expect(mockExport).toHaveBeenCalledWith(
      [{ name: 'id', displayName: 'ID' }],
      'state:ready',
      ['dut-1'],
    );
  });

  it('should handle export errors gracefully', async () => {
    const mockExport = jest.fn().mockRejectedValue(new Error('API Error'));
    const consoleErrorSpy = jest
      .spyOn(console, 'error')
      .mockImplementation(() => {});
    const fakeTable = {
      getVisibleLeafColumns: () => [{ id: 'id', columnDef: { header: 'ID' } }],
      getRowModel: () => ({ rows: [{ id: 'dut-1' }] }),
      getState: () => ({ rowSelection: {} }),
    } as unknown as Parameters<typeof FleetCSVExportButton>[0]['table'];

    render(
      <FakeContextProvider>
        <FleetCSVExportButton
          table={fakeTable}
          filter="state:ready"
          fileName="test_export"
          onExport={mockExport}
        />
      </FakeContextProvider>,
    );

    const exportBtn = screen.getByRole('button', { name: /export/i });
    await userEvent.click(exportBtn);

    const allBtn = await screen.findByText('Export all (CSV)');
    await userEvent.click(allBtn);

    const errorAlert = await screen.findByText(
      /An error occurred during CSV export:.*API Error/i,
    );
    expect(errorAlert).toBeVisible();

    consoleErrorSpy.mockRestore();
  });
});
