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

import '@testing-library/jest-dom';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: jest.fn(),
}));

const mockedUseParams = jest.mocked(jest.requireMock('react-router').useParams);

import { ATP_TEST_NAME_COLUMN } from '@/crystal_ball/constants';
import * as filterApiHooks from '@/crystal_ball/hooks/use_measurement_filter_api';
import {
  MeasurementFilterColumn_ColumnDataType,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { FilterEditor } from './filter_editor';

jest.mock('@/crystal_ball/hooks/use_measurement_filter_api');
const mockedSuggestValues = jest.mocked(
  filterApiHooks.useSuggestMeasurementFilterValues,
);

const mockUUID = 'a-b-c-d-e';
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false },
  },
});

function wrapWithProviders(ui: React.ReactElement) {
  return render(
    <MemoryRouter initialEntries={['/dashboard/test-dashboard']}>
      <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
    </MemoryRouter>,
  );
}
beforeAll(() => {
  jest.spyOn(self.crypto, 'randomUUID').mockReturnValue(mockUUID);
});

afterAll(() => {
  jest.restoreAllMocks();
});

const defaultProps = {
  filters: [] as PerfFilter[],
  onUpdateFilters: jest.fn(),
  dataSpecId: 'test-spec-id',
  availableColumns: [
    {
      column: ATP_TEST_NAME_COLUMN,
      primary: true,
      dataType: MeasurementFilterColumn_ColumnDataType.STRING,
      sampleValues: [],
      isMetricKey: false,
      applicableScopes: [],
    },
    {
      column: 'build_branch',
      primary: false,
      dataType: MeasurementFilterColumn_ColumnDataType.STRING,
      sampleValues: [],
      isMetricKey: false,
      applicableScopes: [],
    },
    {
      column: 'build_target',
      primary: false,
      dataType: MeasurementFilterColumn_ColumnDataType.STRING,
      sampleValues: [],
      isMetricKey: false,
      applicableScopes: [],
    },
    {
      column: 'test_name',
      primary: true,
      dataType: MeasurementFilterColumn_ColumnDataType.STRING,
      sampleValues: [],
      isMetricKey: false,
      applicableScopes: [],
    },
    {
      column: 'model',
      primary: false,
      dataType: MeasurementFilterColumn_ColumnDataType.STRING,
      sampleValues: [],
      isMetricKey: false,
      applicableScopes: [],
    },
    {
      column: 'sku',
      primary: true,
      dataType: MeasurementFilterColumn_ColumnDataType.STRING,
      sampleValues: [],
      isMetricKey: false,
      applicableScopes: [],
    },
  ],
};

describe('FilterEditor', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedUseParams.mockReturnValue({ dashboardId: 'test-dashboard' });
    defaultProps.onUpdateFilters.mockClear();
    mockedSuggestValues.mockClear();
    mockedSuggestValues.mockReturnValue({
      data: { values: ['suggest1', 'suggest2'] },
      isLoading: false,
      isError: false,
    } as unknown as ReturnType<
      typeof filterApiHooks.useSuggestMeasurementFilterValues
    >);
  });

  it('renders with no filters initially', () => {
    wrapWithProviders(<FilterEditor {...defaultProps} />);
    expect(screen.getByText('Filters')).toBeInTheDocument();
    expect(screen.getByText('No filters applied.')).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /Remove filter/i }),
    ).not.toBeInTheDocument();
  });

  it('renders with initial filters', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
        textInput: {
          defaultValue: {
            values: ['initialValue'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );

    // Check chip label when collapsed
    expect(screen.getByText('test_name = "initialValue"')).toBeInTheDocument();

    // Expand the accordion
    fireEvent.click(screen.getByText('Filters'));
    await waitFor(() => {
      expect(screen.getByLabelText('Value')).toBeInTheDocument();
    });

    expect(screen.getByLabelText('Value')).toHaveValue('initialValue');
    expect(screen.getByLabelText('Column')).toHaveTextContent('test_name');
    expect(screen.getByLabelText('Operator')).toHaveTextContent('=');
  });

  it('adds a new filter when "Add Filter" is clicked', async () => {
    wrapWithProviders(<FilterEditor {...defaultProps} />);
    fireEvent.click(screen.getByText('Filters')); // Expand
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Add Filter/i }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: /Add Filter/i }));

    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
    expect(updatedFilters.length).toBe(1);
    expect(updatedFilters[0]).toMatchObject({
      id: `filter-${mockUUID}`,
      column: ATP_TEST_NAME_COLUMN,
      dataSpecId: 'test-spec-id',
      textInput: {
        defaultValue: {
          values: [''],
          filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
        },
      },
    });
  });

  it('removes a filter when delete icon is clicked', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Remove filter/i }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: /Remove filter/i }));

    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    expect(defaultProps.onUpdateFilters).toHaveBeenCalledWith([]);
  });

  it('updates filter column onChange', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
        textInput: {
          defaultValue: {
            values: ['test'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand
    await waitFor(() => {
      expect(screen.getByLabelText('Column')).toBeInTheDocument();
    });

    // interact with the combobox
    fireEvent.mouseDown(screen.getByLabelText('Column'));
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'sku' })).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('option', { name: 'sku' }));

    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
    expect(updatedFilters[0].column).toBe('sku');
  });

  it('updates filter operator onChange', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
        textInput: {
          defaultValue: {
            values: ['test'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand
    await waitFor(() => {
      expect(screen.getByLabelText('Operator')).toBeInTheDocument();
    });

    // interact with the combobox
    fireEvent.mouseDown(screen.getByLabelText('Operator'));
    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'contains' }),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('option', { name: 'contains' }));

    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
    expect(updatedFilters[0].textInput.defaultValue.filterOperator).toBe(
      PerfFilterDefault_FilterOperator.CONTAINS,
    );
  });

  it('updates filter value only onBlur', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
        textInput: {
          defaultValue: {
            values: ['initial'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand
    const valueInput = await screen.findByLabelText('Value');

    fireEvent.change(valueInput, { target: { value: 'newValue' } });
    // onUpdateFilters should not be called yet
    expect(defaultProps.onUpdateFilters).not.toHaveBeenCalled();
    expect(valueInput).toHaveValue('newValue');

    fireEvent.blur(valueInput);
    // Now onUpdateFilters should be called
    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
    expect(updatedFilters[0].textInput.defaultValue.values).toEqual([
      'newValue',
    ]);
  });

  it('does not call onUpdateFilters onBlur if value is unchanged', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
        textInput: {
          defaultValue: {
            values: ['initial'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand
    const valueInput = await screen.findByLabelText('Value');

    fireEvent.focus(valueInput);
    fireEvent.blur(valueInput);
    // onUpdateFilters should not be called as the value didn't change
    expect(defaultProps.onUpdateFilters).not.toHaveBeenCalled();
  });

  it('renders a spinner when isLoadingColumns is true', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
      },
    ];
    wrapWithProviders(
      <FilterEditor
        {...defaultProps}
        filters={initialFilters}
        isLoadingColumns={true}
      />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand
    await waitFor(() => {
      expect(screen.getByRole('progressbar')).toBeInTheDocument();
    });
  });

  it('groups and sorts columns correctly', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
        textInput: {
          defaultValue: {
            values: ['test'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );

    // Expand accordion
    fireEvent.click(screen.getByText('Filters'));

    // Wait for the column combobox to appear
    const columnSelect = await screen.findByLabelText('Column');
    fireEvent.mouseDown(columnSelect);

    // Primary components should be visible and sorted alphabetically
    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'atp_test_name' }),
      ).toBeInTheDocument();
      expect(screen.getAllByRole('option').length).toBeGreaterThan(3);
    });

    const allOptions = screen.getAllByRole('option');
    // First 3 are primary
    expect(allOptions[0]).toHaveTextContent('atp_test_name'); // Primary 1
    expect(allOptions[1]).toHaveTextContent('sku'); // Primary 2
    expect(allOptions[2]).toHaveTextContent('test_name'); // Primary 3
    // Then the divider
    expect(allOptions[3]).toHaveTextContent('');

    const optionsText = allOptions.map((opt) => opt.textContent);

    // First 3 are primary, remaining are secondary (in alphabetical order)
    expect(optionsText).toEqual([
      'atp_test_name',
      'sku',
      'test_name',
      '',
      'build_branch',
      'build_target',
      'model',
    ]);
  });

  it('includes global filters for atp_test_name in suggestions', async () => {
    const globalFilters: PerfFilter[] = [
      {
        id: 'global-filter-1',
        column: ATP_TEST_NAME_COLUMN,
        dataSpecId: 'test-spec-id',
        displayName: 'Global Atp Test Name',
        textInput: {
          defaultValue: {
            values: ['globalValue'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
        textInput: {
          defaultValue: {
            values: ['initial'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    wrapWithProviders(
      <FilterEditor
        {...defaultProps}
        filters={initialFilters}
        globalFilters={globalFilters}
      />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand
    const valueInput = await screen.findByLabelText('Value');

    fireEvent.focus(valueInput);
    fireEvent.change(valueInput, { target: { value: 'newValue' } });

    await waitFor(() => {
      expect(mockedSuggestValues).toHaveBeenCalledWith(
        expect.objectContaining({
          filter: 'atp_test_name = "globalValue"',
        }),
        expect.objectContaining({ enabled: true }),
      );
    });
  });

  it('excludes current filter for atp_test_name from suggestions', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: ATP_TEST_NAME_COLUMN,
        dataSpecId: 'test-spec-id',
        displayName: 'Atp Test Name',
        textInput: {
          defaultValue: {
            values: ['test-value'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand
    const valueInput = await screen.findByLabelText('Value');

    fireEvent.focus(valueInput);
    fireEvent.change(valueInput, { target: { value: 'newValue' } });

    await waitFor(() => {
      expect(mockedSuggestValues).toHaveBeenCalledWith(
        expect.objectContaining({
          filter: '', // Current filter should be excluded, so no filter string
        }),
        expect.objectContaining({ enabled: true }),
      );
    });
  });

  it('disables suggestions when column is not atp_test_name and filter is missing', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name', // Not atp_test_name
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name',
        textInput: {
          defaultValue: {
            values: ['initial'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand
    const valueInput = await screen.findByLabelText('Value');

    fireEvent.focus(valueInput);
    fireEvent.change(valueInput, { target: { value: 'newValue' } });

    await waitFor(() => {
      expect(mockedSuggestValues).toHaveBeenCalledWith(
        expect.any(Object),
        expect.objectContaining({ enabled: false }),
      );
    });
  });

  it('renders with custom title', () => {
    wrapWithProviders(<FilterEditor {...defaultProps} title="Custom Title" />);
    expect(screen.getByText('Custom Title')).toBeInTheDocument();
  });
});
