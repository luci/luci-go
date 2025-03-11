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
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from '@testing-library/react';

import {
  getPageSize,
  getPageToken,
  getPrevFullRowCount,
  usePagerContext,
} from '@/common/components/params_pager';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Device,
  DeviceState,
  DeviceType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { DeviceTable } from './device_table';

const COLUMNS: string[] = ['id', 'dut_id', 'state', 'type'];

const DEFAULT_COLUMNS: string[] = ['id', 'dut_id', 'state'];

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

function TestComponent({
  withKnownTotalRowCount = false,
}: {
  withKnownTotalRowCount?: boolean;
}) {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [3, 5, 10],
    defaultPageSize: 5,
  });

  const totalRowCount = MOCK_DEVICES.length;

  const [searchParams] = useSyncedSearchParams();
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
  return (
    <DeviceTable
      devices={currentDevices}
      columnIds={COLUMNS}
      nextPageToken={nextPageToken}
      pagerCtx={pagerCtx}
      isError={false}
      error={undefined}
      isLoading={false}
      isLoadingColumns={false}
      totalRowCount={withKnownTotalRowCount ? totalRowCount : undefined}
    />
  );
}

const getTableVisibleColumns = () => {
  const grid = screen.getByRole('grid');
  grid.focus();
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

const getPageRowCount = () => {
  return screen
    .getAllByRole('row')
    .filter((row) => row.hasAttribute('data-rowindex')).length;
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

  const newSizeOption = screen
    .getAllByRole('option')
    .find((option) => option.getAttribute('data-value') === String(size))!;
  await act(async () => fireEvent.click(newSizeOption));
};

describe('<DeviceTable />', () => {
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
        routerOptions={{ initialEntries: ['?c=id&c=dut_id'] }}
      >
        <TestComponent />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(getTableVisibleColumns()).toEqual(
      expect.arrayContaining(['id', 'dut_id']),
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
    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('1-5 of more than 5')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();

    await changePageSize(10);

    expect(getNthRow(0)).toHaveAttribute('data-id', '1');
    expect(getPageRowCount()).toBe(10);
    expect(screen.getByText('1-10 of more than 10')).toBeInTheDocument();
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
    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('1-5 of more than 5')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();

    await goToNextPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '6');
    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('6-10 of more than 10')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();

    // Last page
    await goToNextPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '11');
    expect(getPageRowCount()).toBe(3);
    expect(screen.getByText('11-13 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeDisabled();
    expect(getPrevPageButton()).toBeEnabled();

    await goToPrevPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '6');
    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('6-10 of more than 10')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();
  });

  it('should preserve the current pagination state and honor new page size for the subsequent pages if the page size changes', async () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(getNthRow(0)).toHaveAttribute('data-id', '1');
    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('1-5 of more than 5')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeDisabled();

    await goToNextPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '6');
    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('6-10 of more than 10')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();

    // Last page
    await goToNextPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '11');
    expect(getPageRowCount()).toBe(3);
    expect(screen.getByText('11-13 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeDisabled();
    expect(getPrevPageButton()).toBeEnabled();

    await changePageSize(3);

    expect(getNthRow(0)).toHaveAttribute('data-id', '11');
    expect(getPageRowCount()).toBe(3);
    expect(screen.getByText('11-13 of 13')).toBeInTheDocument();
    expect(getNextPageButton()).toBeDisabled();
    expect(getPrevPageButton()).toBeEnabled();

    await goToPrevPage();

    expect(getNthRow(0)).toHaveAttribute('data-id', '6');
    expect(getPageRowCount()).toBe(3);
    expect(screen.getByText('6-8 of more than 8')).toBeInTheDocument();
    expect(getNextPageButton()).toBeEnabled();
    expect(getPrevPageButton()).toBeEnabled();
  }, 6000);

  it('should show total row count in pagination when it is provided', async () => {
    render(
      <FakeContextProvider>
        <TestComponent withKnownTotalRowCount={true} />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(getPageRowCount()).toBe(5);
    expect(screen.getByText('1-5 of 13')).toBeInTheDocument();
  });
});
