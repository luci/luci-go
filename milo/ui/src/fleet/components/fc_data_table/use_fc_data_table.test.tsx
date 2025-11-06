// Copyright 2025 The LUCI Authors.
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

import { error } from 'node:console';

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from '@testing-library/react';
import {
  MaterialReactTable,
  type MRT_ColumnDef,
  MRT_PaginationState,
} from 'material-react-table';
import { useState } from 'react';

import {
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  getPrevFullRowCount,
  nextPageTokenUpdater,
  pageSizeUpdater,
  prevPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { mockVirtualizedListDomProperties } from '@/fleet/testing_tools/dom_mocks';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Device,
  DeviceState,
  DeviceType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useFCDataTable } from './use_fc_data_table';

const COLUMNS: MRT_ColumnDef<Device>[] = [
  {
    accessorKey: 'id',
    header: 'ID',
  },
  {
    accessorKey: 'dutId',
    header: 'DUT ID',
  },
  {
    accessorKey: 'state',
    header: 'State',
  },
  {
    accessorKey: 'type',
    header: 'Type',
  },
];

const MOCK_DEVICES: Device[] = [
  {
    id: '1',
    dutId: 'dut-1',
    type: DeviceType.DEVICE_TYPE_PHYSICAL,
    state: DeviceState.DEVICE_STATE_AVAILABLE,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '2',
    dutId: 'dut-2',
    type: DeviceType.DEVICE_TYPE_VIRTUAL,
    state: DeviceState.DEVICE_STATE_LEASED,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '3',
    dutId: 'dut-3',
    type: DeviceType.DEVICE_TYPE_PHYSICAL,
    state: DeviceState.DEVICE_STATE_AVAILABLE,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '4',
    dutId: 'dut-4',
    type: DeviceType.DEVICE_TYPE_VIRTUAL,
    state: DeviceState.DEVICE_STATE_LEASED,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '5',
    dutId: 'dut-5',
    type: DeviceType.DEVICE_TYPE_PHYSICAL,
    state: DeviceState.DEVICE_STATE_AVAILABLE,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '6',
    dutId: 'dut-6',
    type: DeviceType.DEVICE_TYPE_VIRTUAL,
    state: DeviceState.DEVICE_STATE_LEASED,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '7',
    dutId: 'dut-7',
    type: DeviceType.DEVICE_TYPE_PHYSICAL,
    state: DeviceState.DEVICE_STATE_AVAILABLE,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '8',
    dutId: 'dut-8',
    type: DeviceType.DEVICE_TYPE_VIRTUAL,
    state: DeviceState.DEVICE_STATE_LEASED,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '9',
    dutId: 'dut-9',
    type: DeviceType.DEVICE_TYPE_PHYSICAL,
    state: DeviceState.DEVICE_STATE_AVAILABLE,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '10',
    dutId: 'dut-10',
    type: DeviceType.DEVICE_TYPE_VIRTUAL,
    state: DeviceState.DEVICE_STATE_LEASED,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '11',
    dutId: 'dut-11',
    type: DeviceType.DEVICE_TYPE_PHYSICAL,
    state: DeviceState.DEVICE_STATE_AVAILABLE,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '12',
    dutId: 'dut-12',
    type: DeviceType.DEVICE_TYPE_VIRTUAL,
    state: DeviceState.DEVICE_STATE_LEASED,
    address: undefined,
    deviceSpec: undefined,
  },
  {
    id: '13',
    dutId: 'dut-13',
    type: DeviceType.DEVICE_TYPE_PHYSICAL,
    state: DeviceState.DEVICE_STATE_AVAILABLE,
    address: undefined,
    deviceSpec: undefined,
  },
];

const DEFAULT_PAGE_SIZE_OPTIONS = [5, 10, 25, 50, 100, 500, 1000];

function TestComponent() {
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: 5,
  });

  const totalRowCount = MOCK_DEVICES.length;

  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const pageToken = getPageToken(pagerCtx, searchParams);
  const pageSize = getPageSize(pagerCtx, searchParams);

  // Consider pageToken is simply page's first element index in MOCK_ROWS
  const currentIndex = Number(pageToken);
  const currentDevices = MOCK_DEVICES.slice(
    currentIndex,
    currentIndex + pageSize,
  );
  const currentRowCount = getPrevFullRowCount(pagerCtx) + currentDevices.length;
  const nextPageToken =
    currentRowCount < totalRowCount ? String(currentRowCount) : '';

  const [pagination, setPagination] = useState<MRT_PaginationState>({
    pageIndex: 0,
    pageSize: getPageSize(pagerCtx, searchParams),
  });

  const table = useFCDataTable({
    columns: COLUMNS,
    data: currentDevices,
    state: {
      pagination: pagination,
    },
    muiPaginationProps: {
      rowsPerPageOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    },

    // Pagination
    rowCount: totalRowCount,
    manualPagination: true,
    onPaginationChange: (updater) => {
      const newPagination =
        typeof updater === 'function' ? updater(pagination) : updater;

      setSearchParams(pageSizeUpdater(pagerCtx, newPagination.pageSize));
      const currentPage = getCurrentPageIndex(pagerCtx);
      const isPrevPage = newPagination.pageIndex < currentPage;
      const isNextPage = newPagination.pageIndex > currentPage;

      if (isPrevPage) {
        setSearchParams(prevPageTokenUpdater(pagerCtx));
      } else if (isNextPage) {
        setSearchParams(nextPageTokenUpdater(pagerCtx, nextPageToken));
      }

      setPagination(newPagination);
    },
  });

  return <MaterialReactTable table={table} />;
}

const mockFleetConsoleClient = {
  ExportDevicesToCSV: {
    query: jest.fn().mockReturnValue({
      queryKey: ['todos'],
      queryFn: jest.fn(),
    }),
  },
};

jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useFleetConsoleClient: () => mockFleetConsoleClient,
}));

const getTableVisibleColumns = () => {
  const columnIdMap = new Map(COLUMNS.map((c) => [c.header, c.accessorKey]));
  const headers = screen.getAllByRole('columnheader');
  return headers
    .map((h) => {
      const headerText = h.querySelector('.Mui-TableHeadCell-Content-Wrapper');
      return headerText !== null && headerText.textContent !== null
        ? columnIdMap.get(headerText.textContent)
        : null;
    })
    .filter((id): id is string => !!id);
};

const getPageRowCount = () => {
  return screen
    .getAllByRole('row')
    .filter((row) => row.hasAttribute('data-index')).length;
};

const getNextPageButton = () => {
  return screen.getByLabelText('Go to next page', { selector: 'button' });
};

const getPrevPageButton = () => {
  return screen.getByLabelText('Go to previous page', { selector: 'button' });
};

const goToNextPage = async () => {
  await act(async () => fireEvent.click(getNextPageButton()));
};

const goToPrevPage = async () => {
  await act(async () => fireEvent.click(getPrevPageButton()));
};

const changePageSize = async (size: number) => {
  await act(async () =>
    fireEvent.mouseDown(
      screen.getByLabelText('Rows per page', { selector: 'div' }),
    ),
  );

  const newSizeOption = screen
    .getAllByRole('option')
    .find((option) => option.getAttribute('data-value') === String(size))!;
  await act(async () => fireEvent.click(newSizeOption));
};

describe('<MaterialReactTable />', () => {
  let cleanupDomMocks: () => void;

  beforeEach(() => {
    jest.useFakeTimers();
    cleanupDomMocks = mockVirtualizedListDomProperties();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.clearAllMocks();
    cleanup();
    cleanupDomMocks();
  });

  it('should render all columns', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());
    error(getTableVisibleColumns());

    expect(getTableVisibleColumns()).toEqual(COLUMNS.map((c) => c.accessorKey));
  });

  it('should reflect page size change properly', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('1-5 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();

    await changePageSize(10);

    expect(getPageRowCount()).toBe(10);
    expect(screen.getByText('1-10 of 13')).toBeInTheDocument();
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

    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('1-5 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();

    await goToNextPage();

    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('6-10 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();

    // Last page
    await goToNextPage();

    expect(getPageRowCount()).toBe(3);
    expect(screen.getByText('11-13 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeDisabled();
    expect(getPrevPageButton()).toBeEnabled();

    await goToPrevPage();

    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('6-10 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();
  });
});
