// Copyright 2024 The LUCI Authors.
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

import {
  GridColDef,
  GridColumnVisibilityModel,
  GridSortModel,
} from '@mui/x-data-grid';
import {
  screen,
  render,
  cleanup,
  act,
  fireEvent,
} from '@testing-library/react';
import { useState } from 'react';

import {
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { DataTable } from './data_table';

const COLUMNS: GridColDef[] = Object.entries({
  id: 'id',
  first_name: 'First Name',
  last_name: 'Last Name',
}).map(([id, displayName]) => ({
  field: id,
  headerName: displayName,
  editable: false,
  minWidth: 70,
  maxWidth: 700,
}));

const DEFAULT_COLUMNS: string[] = ['id', 'first_name'];

const MOCK_ROWS: { [key: string]: string }[] = [
  { id: '1', first_name: 'Alice', last_name: 'Smith' },
  { id: '2', first_name: 'Bob', last_name: 'Johnson' },
  { id: '3', first_name: 'Charlie', last_name: 'Williams' },
  { id: '4', first_name: 'David', last_name: 'Brown' },
  { id: '5', first_name: 'Emily', last_name: 'Jones' },
  { id: '6', first_name: 'Frank', last_name: 'Miller' },
  { id: '7', first_name: 'Grace', last_name: 'Davis' },
  { id: '8', first_name: 'Henry', last_name: 'Garcia' },
  { id: '9', first_name: 'Isabella', last_name: 'Rodriguez' },
  { id: '10', first_name: 'Jack', last_name: 'Wilson' },
  { id: '11', first_name: 'Katie', last_name: 'Martinez' },
  { id: '12', first_name: 'Liam', last_name: 'Anderson' },
  { id: '13', first_name: 'Mia', last_name: 'Taylor' },
];

function TestComponent() {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [3, 5, 10],
    defaultPageSize: 5,
  });

  const totalRowCount = MOCK_ROWS.length;

  const [searchParams] = useSyncedSearchParams();
  const pageToken = getPageToken(pagerCtx, searchParams);
  const pageSize = getPageSize(pagerCtx, searchParams);
  const currentPageIndex = getCurrentPageIndex(pagerCtx);
  const [sortModel, setSortModel] = useState<GridSortModel>([]);

  // Consider pageToken is simply page's first element index in MOCK_ROWS
  const currentIndex = Number(pageToken);
  const currentRows = MOCK_ROWS.slice(currentIndex, currentIndex + pageSize);
  const currentRowCount = pageSize * currentPageIndex + currentRows.length;
  const nextPageToken =
    currentRowCount < totalRowCount ? String(currentRowCount) : '';
  return (
    <DataTable
      defaultColumnVisibilityModel={COLUMNS.reduce(
        (visibilityModel, column) => ({
          ...visibilityModel,
          [column.field]: DEFAULT_COLUMNS.includes(column.field),
        }),
        {} as GridColumnVisibilityModel,
      )}
      columns={COLUMNS}
      rows={currentRows}
      nextPageToken={nextPageToken}
      isLoading={false}
      pagerCtx={pagerCtx}
      sortModel={sortModel}
      onSortModelChange={setSortModel}
    />
  );
}

const getTableVisibleColumns = () => {
  return screen
    .getAllByRole('columnheader')
    .map((element) => element.getAttribute('data-field'))
    .filter((column) => column !== null);
};

const getNthRow = (index: number) => {
  return screen
    .getAllByRole('row')
    .find((row) => row.getAttribute('data-rowindex') === String(index));
};

const getNextPageButton = () => {
  return screen.getByLabelText('Go to next page');
};

const getPrevPageButton = () => {
  return screen.getByLabelText('Go to previous page');
};

const goToNextPage = async () => {
  await act(async () => fireEvent.click(getNextPageButton()));
};

const goToPrevPage = async () => {
  await act(async () => fireEvent.click(getPrevPageButton()));
};

const changePageSize = async (size: number) => {
  await act(async () =>
    fireEvent.mouseDown(screen.getByLabelText('Rows per page:')),
  );

  const new_size_option = screen
    .getAllByRole('option')
    .find((option) => option.getAttribute('data-value') === String(size))!;
  await act(async () => fireEvent.click(new_size_option));
};

describe('<DataTable />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    cleanup();
  });

  it('should start with default columns when no specified columns in the url', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(getTableVisibleColumns()).toEqual(
      expect.arrayContaining(DEFAULT_COLUMNS),
    );
  });

  it('should start with columns specified in the url', async () => {
    render(
      <FakeContextProvider
        routerOptions={{ initialEntries: ['?c=id&c=last_name'] }}
      >
        <TestComponent />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(getTableVisibleColumns()).toEqual(
      expect.arrayContaining(['id', 'last_name']),
    );
  });

  it('should reflect page size change properly', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(getNthRow(0)).toHaveAttribute('data-id', '1');
    expect(getNthRow(4)).toBeInTheDocument();
    expect(getNthRow(5)).toBeUndefined();
    expect(screen.getByText('1–5 of more than 5')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();

    await changePageSize(10);

    expect(getNthRow(0)).toHaveAttribute('data-id', '1');
    expect(getNthRow(9)).toBeInTheDocument();
    expect(getNthRow(10)).toBeUndefined();
    expect(screen.getByText('1–10 of more than 10')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();
  });

  it('should navigate between pages properly', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(getNthRow(0)).toHaveAttribute('data-id', '1');
    expect(screen.getByText('1–5 of more than 5')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();

    await goToNextPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '6');
    expect(screen.getByText('6–10 of more than 10')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();

    // Last page.
    await goToNextPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '11');
    expect(screen.getByText('11–13 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeDisabled();
    expect(getPrevPageButton()).toBeEnabled();

    await goToPrevPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '6');
    expect(screen.getByText('6–10 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();
  });

  // TODO(vaghinak): There is an issue with changing the page size in the middle pages.
  // Based on aip-158 if user changes the page size we should stay on the same page,
  // but it turns out mui x DataGrid goes to the first page when total data count is
  // not known. Should remove `skip` when it is fixed.
  it.skip('should preserve the page when changing page size in the middle pages', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(getNthRow(0)).toHaveAttribute('data-id', '1');
    expect(screen.getByText('1–5 of more than 5')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();

    await goToNextPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '6');
    expect(screen.getByText('6–10 of more than 10')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();

    await changePageSize(3);

    expect(getNthRow(0)).toHaveAttribute('data-id', '6');
    expect(screen.getByText('6–8 of more than 8')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();

    await goToPrevPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '1');
    expect(screen.getByText('1–5 of more than 5')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();
  });
});
