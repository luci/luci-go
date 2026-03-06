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

import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import { PerfFilter } from '@/crystal_ball/types';

import { FilterEditor } from './filter_editor';

const mockUUID = 'a-b-c-d-e';
beforeAll(() => {
  if (!self.crypto.randomUUID) {
    self.crypto.randomUUID = () => mockUUID;
  }
});

afterAll(() => {
  jest.restoreAllMocks();
});

const defaultProps = {
  filters: [] as PerfFilter[],
  onUpdateFilters: jest.fn(),
  dataSpecId: 'test-spec-id',
};

describe('FilterEditor', () => {
  beforeEach(() => {
    defaultProps.onUpdateFilters.mockClear();
  });

  it('renders with no filters initially', () => {
    render(<FilterEditor {...defaultProps} />);
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
        textInput: {
          defaultValue: { values: ['initialValue'], filterOperator: 'EQUAL' },
        },
      },
    ];
    render(<FilterEditor {...defaultProps} filters={initialFilters} />);

    // Check chip label when collapsed
    expect(
      screen.getByText('test_name EQUAL "initialValue"'),
    ).toBeInTheDocument();

    // Expand the accordion
    fireEvent.click(screen.getByText('Filters'));
    await waitFor(() => {
      expect(screen.getByLabelText('Value')).toBeInTheDocument();
    });

    expect(screen.getByLabelText('Value')).toHaveValue('initialValue');
    expect(screen.getByLabelText('Column')).toHaveTextContent('test_name');
    expect(screen.getByLabelText('Operator')).toHaveTextContent('EQUAL');
  });

  it('adds a new filter when "Add Filter" is clicked', async () => {
    render(<FilterEditor {...defaultProps} />);
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
      column: 'atp_test_name',
      dataSpecId: 'test-spec-id',
      textInput: {
        defaultValue: { values: [''], filterOperator: 'EQUAL' },
      },
    });
  });

  it('removes a filter when delete icon is clicked', async () => {
    const initialFilters: PerfFilter[] = [
      { id: 'filter-1', column: 'test_name', dataSpecId: 'test-spec-id' },
    ];
    render(<FilterEditor {...defaultProps} filters={initialFilters} />);
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
        textInput: {
          defaultValue: { values: ['test'], filterOperator: 'EQUAL' },
        },
      },
    ];
    render(<FilterEditor {...defaultProps} filters={initialFilters} />);
    fireEvent.click(screen.getByText('Filters')); // Expand
    await waitFor(() => {
      expect(screen.getByLabelText('Column')).toBeInTheDocument();
    });

    // interact with the combobox
    fireEvent.mouseDown(screen.getByLabelText('Column'));
    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'build_branch' }),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('option', { name: 'build_branch' }));

    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
    expect(updatedFilters[0].column).toBe('build_branch');
  });

  it('updates filter operator onChange', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        textInput: {
          defaultValue: { values: ['test'], filterOperator: 'EQUAL' },
        },
      },
    ];
    render(<FilterEditor {...defaultProps} filters={initialFilters} />);
    fireEvent.click(screen.getByText('Filters')); // Expand
    await waitFor(() => {
      expect(screen.getByLabelText('Operator')).toBeInTheDocument();
    });

    // interact with the combobox
    fireEvent.mouseDown(screen.getByLabelText('Operator'));
    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'CONTAINS' }),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('option', { name: 'CONTAINS' }));

    expect(defaultProps.onUpdateFilters).toHaveBeenCalledTimes(1);
    const updatedFilters = defaultProps.onUpdateFilters.mock.calls[0][0];
    expect(updatedFilters[0].textInput.defaultValue.filterOperator).toBe(
      'CONTAINS',
    );
  });

  it('updates filter value only onBlur', async () => {
    const initialFilters: PerfFilter[] = [
      {
        id: 'filter-1',
        column: 'test_name',
        dataSpecId: 'test-spec-id',
        textInput: {
          defaultValue: { values: ['initial'], filterOperator: 'EQUAL' },
        },
      },
    ];
    render(<FilterEditor {...defaultProps} filters={initialFilters} />);
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
        textInput: {
          defaultValue: { values: ['initial'], filterOperator: 'EQUAL' },
        },
      },
    ];
    render(<FilterEditor {...defaultProps} filters={initialFilters} />);
    fireEvent.click(screen.getByText('Filters')); // Expand
    const valueInput = await screen.findByLabelText('Value');

    fireEvent.focus(valueInput);
    fireEvent.blur(valueInput);
    // onUpdateFilters should not be called as the value didn't change
    expect(defaultProps.onUpdateFilters).not.toHaveBeenCalled();
  });
});
