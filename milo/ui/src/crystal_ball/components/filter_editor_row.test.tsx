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

import { fireEvent, render, screen } from '@testing-library/react';
import { useParams } from 'react-router';

import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { useSuggestMeasurementFilterValues } from '@/crystal_ball/hooks';
import { createMockQueryResult } from '@/crystal_ball/tests';
import {
  MeasurementFilterColumn_ColumnDataType,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { FilterEditorRow } from './filter_editor_row';

const mockCopyFilters = jest.fn();
jest.mock('@/crystal_ball/hooks', () => ({
  ...jest.requireActual('@/crystal_ball/hooks'),
  useSuggestMeasurementFilterValues: jest.fn(),
  useFiltersClipboard: () => ({
    copyFilters: mockCopyFilters,
  }),
}));
const mockSuggest = jest.mocked(useSuggestMeasurementFilterValues);

jest.mock('react-router', () => ({
  useParams: jest.fn(),
}));
const mockedUseParams = jest.mocked(useParams);

describe('FilterEditorRow', () => {
  const defaultProps = {
    filter: {
      id: 'filter-1',
      column: 'test_name',
      dataSpecId: 'spec-1',
      displayName: 'Test Name',
      textInput: {
        defaultValue: {
          values: ['value1'],
          filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
        },
      },
    } as PerfFilter,
    dataSpecId: 'spec-1',
    primaryColumns: ['test_name', 'build_branch'],
    secondaryColumns: ['model'],
    dataType: MeasurementFilterColumn_ColumnDataType.STRING,
    onUpdateColumn: jest.fn(),
    onUpdateOperator: jest.fn(),
    onUpdateValue: jest.fn(),
    onRemove: jest.fn(),
    onDragStart: jest.fn(),
    onDrop: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
    mockSuggest.mockReturnValue(createMockQueryResult({ values: [] }));
    mockedUseParams.mockReturnValue({ dashboardId: 'dash-1' });
  });

  it('renders with initial values', () => {
    render(<FilterEditorRow {...defaultProps} />);

    // Check column select
    expect(
      screen.getByRole('combobox', { name: COMMON_MESSAGES.COLUMN }),
    ).toHaveTextContent('test_name');

    // Check operator select
    // MUI Select renders a div for display value when not expanded
    expect(
      screen.getByRole('combobox', { name: COMMON_MESSAGES.OPERATOR }),
    ).toHaveTextContent('=');

    // Check value input
    expect(
      screen.getByRole('combobox', { name: COMMON_MESSAGES.VALUE }),
    ).toHaveValue('value1');
  });

  it('calls onUpdateColumn when column changes', () => {
    render(<FilterEditorRow {...defaultProps} />);

    const columnSelect = screen.getByRole('combobox', {
      name: COMMON_MESSAGES.COLUMN,
    });
    fireEvent.mouseDown(columnSelect);

    const option = screen.getByRole('option', { name: 'build_branch' });
    fireEvent.click(option);

    expect(defaultProps.onUpdateColumn).toHaveBeenCalledWith('build_branch');
  });

  it('calls onUpdateOperator when operator changes', () => {
    render(<FilterEditorRow {...defaultProps} />);

    const operatorSelect = screen.getByRole('combobox', {
      name: COMMON_MESSAGES.OPERATOR,
    });
    fireEvent.mouseDown(operatorSelect);

    const option = screen.getByRole('option', { name: '!=' });
    fireEvent.click(option);

    expect(defaultProps.onUpdateOperator).toHaveBeenCalledWith(
      PerfFilterDefault_FilterOperator.NOT_EQUAL,
    );
  });

  it('calls onUpdateValue on blur if value changed', () => {
    render(<FilterEditorRow {...defaultProps} />);

    const valueInput = screen.getByRole('combobox', {
      name: COMMON_MESSAGES.VALUE,
    });
    fireEvent.change(valueInput, { target: { value: 'newValue' } });
    fireEvent.blur(valueInput);

    expect(defaultProps.onUpdateValue).toHaveBeenCalledWith('newValue');
  });

  it('calls onRemove when delete button is clicked', () => {
    render(<FilterEditorRow {...defaultProps} />);

    const deleteButton = screen.getByRole('button', { name: 'Remove filter' });
    fireEvent.click(deleteButton);

    expect(defaultProps.onRemove).toHaveBeenCalled();
  });

  it('calls copyFilters when copy button is clicked', () => {
    render(<FilterEditorRow {...defaultProps} />);

    const copyButton = screen.getByRole('button', {
      name: COMMON_MESSAGES.COPY_FILTER,
    });
    fireEvent.click(copyButton);

    expect(mockCopyFilters).toHaveBeenCalledWith([defaultProps.filter]);
  });
});
