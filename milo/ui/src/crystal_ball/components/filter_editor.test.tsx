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
import { useState } from 'react';
import { MemoryRouter } from 'react-router';

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: jest.fn(),
}));

jest.mock('@mui/material', () => ({
  ...jest.requireActual('@mui/material'),
  Collapse: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

const mockShowSuccessToast = jest.fn();
const mockShowWarningToast = jest.fn();

jest.mock('@/crystal_ball/hooks', () => ({
  ...jest.requireActual('@/crystal_ball/hooks'),
  useEditorUiState: ({ initialValue = false }: UseEditorUiStateOptions) => {
    const [val, setVal] = useState(initialValue);
    return [val, setVal];
  },
  useToast: () => ({
    showSuccessToast: mockShowSuccessToast,
    showWarningToast: mockShowWarningToast,
    showErrorToast: jest.fn(),
  }),
}));

const mockedUseParams = jest.mocked(jest.requireMock('react-router').useParams);

import { Column, COMMON_MESSAGES } from '@/crystal_ball/constants';
import { UseEditorUiStateOptions } from '@/crystal_ball/hooks';
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
      column: Column.ATP_TEST_NAME,
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
    localStorage.clear();
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
    expect(screen.getAllByText('No filters applied.')).toHaveLength(2);
    expect(screen.getByTestId('add-filter-button-top')).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /Remove filter/i }),
    ).not.toBeInTheDocument();
  });

  it('renders placeholder when expanded and empty', () => {
    wrapWithProviders(<FilterEditor {...defaultProps} />);
    fireEvent.click(screen.getByText('Filters')); // Expand
    expect(screen.getByText('No filters applied.')).toBeInTheDocument();
  });

  it('renders "Add" button even when collapsed', () => {
    wrapWithProviders(<FilterEditor {...defaultProps} />);
    expect(screen.getByTestId('add-filter-button-top')).toBeInTheDocument();
    expect(screen.getByTestId('add-filter-button-top')).toHaveTextContent(
      'Add',
    );
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
      expect(screen.getByLabelText(COMMON_MESSAGES.VALUE)).toBeInTheDocument();
    });

    expect(screen.getByLabelText(COMMON_MESSAGES.VALUE)).toHaveValue(
      'initialValue',
    );
    expect(screen.getByLabelText(COMMON_MESSAGES.COLUMN)).toHaveTextContent(
      'test_name',
    );
    expect(screen.getByLabelText(COMMON_MESSAGES.OPERATOR)).toHaveTextContent(
      '=',
    );
  });

  it('adds a new filter when "Add" button is clicked when empty', async () => {
    wrapWithProviders(<FilterEditor {...defaultProps} />);

    // Button should be visible
    expect(screen.getByTestId('add-filter-button-top')).toBeInTheDocument();

    // Click "Add" button
    fireEvent.click(screen.getByTestId('add-filter-button-top'));

    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
    expect(updatedFilters.length).toBe(1);
    expect(updatedFilters[0]).toMatchObject({
      id: `filter-${mockUUID}`,
      column: Column.ATP_TEST_NAME,
      dataSpecId: 'test-spec-id',
      textInput: {
        defaultValue: {
          values: [''],
          filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
        },
      },
    });
  });

  it('adds a new filter when "Add" button is clicked and expands', async () => {
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

    // Button should be visible even when collapsed
    expect(screen.getByTestId('add-filter-button-top')).toBeInTheDocument();

    // Click "Add" button while collapsed
    fireEvent.click(screen.getByTestId('add-filter-button-top'));

    // Should add a filter
    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
    expect(updatedFilters.length).toBe(2);

    // Should expand accordion (Value input should become visible)
    await waitFor(() => {
      expect(screen.getByLabelText(COMMON_MESSAGES.VALUE)).toBeInTheDocument();
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
      expect(screen.getByLabelText(COMMON_MESSAGES.COLUMN)).toBeInTheDocument();
    });

    // interact with the combobox
    fireEvent.mouseDown(screen.getByLabelText(COMMON_MESSAGES.COLUMN));
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
      expect(
        screen.getByLabelText(COMMON_MESSAGES.OPERATOR),
      ).toBeInTheDocument();
    });

    // interact with the combobox
    fireEvent.mouseDown(screen.getByLabelText(COMMON_MESSAGES.OPERATOR));
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
    const valueInput = await screen.findByLabelText(COMMON_MESSAGES.VALUE);

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
    const valueInput = await screen.findByLabelText(COMMON_MESSAGES.VALUE);

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
    const columnSelect = await screen.findByLabelText(COMMON_MESSAGES.COLUMN);
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
        column: Column.ATP_TEST_NAME,
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
    const valueInput = await screen.findByLabelText(COMMON_MESSAGES.VALUE);

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
        column: Column.ATP_TEST_NAME,
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
    const valueInput = await screen.findByLabelText(COMMON_MESSAGES.VALUE);

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
    const valueInput = await screen.findByLabelText(COMMON_MESSAGES.VALUE);

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

  it('moves a filter up when dragged', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name 1',
      },
      {
        id: 'filter-2',
        column: 'model',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name 2',
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand

    const columns = await screen.findAllByLabelText(COMMON_MESSAGES.COLUMN);
    const sourceRow = columns[1].closest('div[draggable="true"]');
    const targetRow = columns[0].closest('div[draggable="true"]');

    expect(sourceRow).toBeTruthy();
    expect(targetRow).toBeTruthy();

    fireEvent.dragStart(sourceRow!);
    fireEvent.drop(targetRow!);

    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    expect(defaultProps.onUpdateFilters).toHaveBeenLastCalledWith([
      expect.objectContaining({ id: 'filter-2' }),
      expect.objectContaining({ id: 'filter-1' }),
    ]);
  });

  it('moves a filter down when dragged', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name 1',
      },
      {
        id: 'filter-2',
        column: 'model',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name 2',
      },
    ];
    wrapWithProviders(
      <FilterEditor {...defaultProps} filters={initialFilters} />,
    );
    fireEvent.click(screen.getByText('Filters')); // Expand

    const columns = await screen.findAllByLabelText(COMMON_MESSAGES.COLUMN);
    const sourceRow = columns[0].closest('div[draggable="true"]');
    const targetRow = columns[1].closest('div[draggable="true"]');

    expect(sourceRow).toBeTruthy();
    expect(targetRow).toBeTruthy();

    fireEvent.dragStart(sourceRow!);
    fireEvent.drop(targetRow!);

    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    expect(defaultProps.onUpdateFilters).toHaveBeenLastCalledWith([
      expect.objectContaining({ id: 'filter-2' }),
      expect.objectContaining({ id: 'filter-1' }),
    ]);
  });

  describe('Copy/Paste functionality', () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        displayName: 'Test Name 1',
      },
    ];

    it('copies filters to localStorage', async () => {
      wrapWithProviders(
        <FilterEditor {...defaultProps} filters={initialFilters} />,
      );
      fireEvent.click(screen.getByText('Filters')); // Expand

      const copyButton = screen.getByRole('button', {
        name: 'Copy all filters',
      });
      fireEvent.click(copyButton);

      const stored = localStorage.getItem('crystal_ball_filters_clipboard');
      expect(stored).toBeTruthy();
      expect(JSON.parse(stored!)).toEqual(initialFilters);
    });

    it('disables paste button when clipboard is empty', async () => {
      wrapWithProviders(<FilterEditor {...defaultProps} />);
      const pasteButton = screen.getByRole('button', { name: 'Paste filters' });
      expect(pasteButton).toBeDisabled();
    });

    it('enables paste button and shows count when clipboard has items', async () => {
      localStorage.setItem(
        'crystal_ball_filters_clipboard',
        JSON.stringify(initialFilters),
      );
      wrapWithProviders(<FilterEditor {...defaultProps} />);

      const pasteButton = screen.getByRole('button', { name: 'Paste filters' });
      expect(pasteButton).not.toBeDisabled();

      const badge = screen.getByText('1');
      expect(badge).toBeInTheDocument();
    });

    it('pastes and appends filters', async () => {
      localStorage.setItem(
        'crystal_ball_filters_clipboard',
        JSON.stringify(initialFilters),
      );
      const existingFilters: PerfFilter[] = [
        {
          id: 'filter-existing',
          column: 'model',
          dataSpecId: 'test-spec-id',
          displayName: 'Existing Filter',
        },
      ];
      wrapWithProviders(
        <FilterEditor {...defaultProps} filters={existingFilters} />,
      );

      const pasteButton = screen.getByRole('button', { name: 'Paste filters' });
      fireEvent.click(pasteButton);

      const appendItem = screen.getByText('Append filters');
      fireEvent.click(appendItem);

      expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
      const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
      expect(updatedFilters).toHaveLength(2);
      expect(updatedFilters[0].id).toBe('filter-existing');
      expect(updatedFilters[1].column).toBe('test_name');
    });

    it('pastes and replaces filters via menu', async () => {
      localStorage.setItem(
        'crystal_ball_filters_clipboard',
        JSON.stringify(initialFilters),
      );
      const existingFilters: PerfFilter[] = [
        {
          id: 'filter-existing',
          column: 'model',
          dataSpecId: 'test-spec-id',
          displayName: 'Existing Filter',
        },
      ];
      wrapWithProviders(
        <FilterEditor {...defaultProps} filters={existingFilters} />,
      );

      const pasteButton = screen.getByRole('button', { name: 'Paste filters' });
      fireEvent.click(pasteButton);

      const replaceItem = screen.getByText('Replace all filters');
      fireEvent.click(replaceItem);

      expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
      const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
      expect(updatedFilters).toHaveLength(1);
      expect(updatedFilters[0].column).toBe('test_name');
    });

    it('clears clipboard via menu', async () => {
      localStorage.setItem(
        'crystal_ball_filters_clipboard',
        JSON.stringify(initialFilters),
      );
      wrapWithProviders(<FilterEditor {...defaultProps} />);

      const pasteButton = screen.getByRole('button', { name: 'Paste filters' });
      fireEvent.click(pasteButton);

      const clearItem = screen.getByText('Clear copied filters');
      fireEvent.click(clearItem);

      expect(localStorage.getItem('crystal_ball_filters_clipboard')).toBeNull();
      expect(mockShowSuccessToast).toHaveBeenCalledWith('Clipboard cleared');
    });

    it('filters out incompatible filters on paste', async () => {
      localStorage.setItem(
        'crystal_ball_filters_clipboard',
        JSON.stringify([
          ...initialFilters,
          {
            id: 'filter-invalid',
            column: 'non-existent-column',
            dataSpecId: 'test-spec-id',
            displayName: 'Invalid Filter',
          },
        ]),
      );
      wrapWithProviders(<FilterEditor {...defaultProps} />);

      const pasteButton = screen.getByRole('button', { name: 'Paste filters' });
      fireEvent.click(pasteButton);

      const appendItem = screen.getByText('Append filters');
      fireEvent.click(appendItem);

      expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
      const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
      expect(updatedFilters).toHaveLength(1);
      expect(updatedFilters[0].column).toBe('test_name');
      expect(mockShowWarningToast).toHaveBeenCalledWith(
        '1 filter(s) pasted. 1 filter(s) ignored.',
      );
    });

    it('clears all filters via confirm dialog', async () => {
      wrapWithProviders(
        <FilterEditor {...defaultProps} filters={initialFilters} />,
      );
      fireEvent.click(screen.getByText('Filters')); // Expand

      const clearButton = screen.getByRole('button', {
        name: 'Clear all filters',
      });
      fireEvent.click(clearButton);

      const confirmButton = screen.getByTestId('confirm-dialog-confirm');
      fireEvent.click(confirmButton);

      expect(defaultProps.onUpdateFilters).toHaveBeenCalledWith([]);
    });
  });
});
