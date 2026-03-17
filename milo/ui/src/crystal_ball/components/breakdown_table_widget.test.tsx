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

import { render, screen, fireEvent } from '@testing-library/react';

import { COMMON_MESSAGES } from '@/crystal_ball/constants/messages';
import { BreakdownTableData } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { BreakdownTableWidget } from './breakdown_table_widget';

describe('BreakdownTableWidget', () => {
  it(`renders "${COMMON_MESSAGES.NO_DATA_AVAILABLE}" when sections are empty`, () => {
    const data = BreakdownTableData.fromPartial({ sections: [] });
    render(<BreakdownTableWidget data={data} />);
    expect(
      screen.getByText(COMMON_MESSAGES.NO_DATA_AVAILABLE),
    ).toBeInTheDocument();
  });

  it('renders the tabs and table when data is provided', () => {
    const data = {
      sections: [
        {
          dimensionColumn: 'testname',
          rows: [{ dimension_value: 'test1', COUNT: 10, MIN: 1, MAX: 5 }],
        },
      ],
    };
    render(<BreakdownTableWidget data={data} />);
    // Check that the dropdown displays the dimension title
    expect(
      screen.getByRole('combobox', { name: 'Breakdown by category' }),
    ).toHaveTextContent(/TESTNAME/i);
    expect(screen.getByText('test1')).toBeInTheDocument();
  });

  it('switches categories and displays corresponding data', () => {
    const data = {
      sections: [
        {
          dimensionColumn: 'testname',
          rows: [{ dimension_value: 'test1', COUNT: 10, MIN: 1, MAX: 5 }],
        },
        {
          dimensionColumn: 'buildbranch',
          rows: [{ dimension_value: 'branch1', COUNT: 20, MIN: 2, MAX: 6 }],
        },
      ],
    };
    render(<BreakdownTableWidget data={data} />);

    // Initial state (first category active check by verifying the select button text)
    expect(
      screen.getByRole('combobox', { name: 'Breakdown by category' }),
    ).toHaveTextContent(/TESTNAME/i);
    expect(screen.getByText('test1')).toBeInTheDocument();
    expect(screen.getByText('10.00')).toBeInTheDocument();

    // Click Category dropdown (MUI Select handles this as a button/listbox combo)
    const select = screen.getByRole('combobox', {
      name: 'Breakdown by category',
    });
    fireEvent.mouseDown(select);

    // Wait for listbox to render and click option
    const listbox = screen.getByRole('listbox');
    fireEvent.click(listbox.childNodes[1]); // Click the second option (BUILDBRANCH)

    // Second category data should be visible
    expect(
      screen.getByRole('combobox', { name: 'Breakdown by category' }),
    ).toHaveTextContent(/BUILDBRANCH/i);
    expect(screen.getByText('branch1')).toBeInTheDocument();
    expect(screen.getByText('20.00')).toBeInTheDocument();
  });

  it('dynamically derives metric columns from data', () => {
    const data = {
      sections: [
        {
          dimensionColumn: 'testname',
          rows: [
            {
              dimension_value: 'test1',
              COUNT: 1,
              PASSES: 50,
              FAILURES: 5,
              CUSTOMSCORE: 9.9,
            },
          ],
        },
      ],
    };
    render(<BreakdownTableWidget data={data} />);
    expect(screen.getByText('PASSES')).toBeInTheDocument();
    expect(screen.getByText('FAILURES')).toBeInTheDocument();
    expect(screen.getByText('CUSTOMSCORE')).toBeInTheDocument(); // Tests upper casing without replacing inner characters
  });

  it('resets pagination state to page 0 across category switches', () => {
    // Deterministic sorting with decreasing counts
    const rows1 = Array.from({ length: 15 }, (_, i) => ({
      dimension_value: `test${i}`,
      COUNT: 100 - i,
    }));
    const rows2 = Array.from({ length: 15 }, (_, i) => ({
      dimension_value: `branch${i}`,
      COUNT: 200 - i,
    }));

    const data = {
      sections: [
        { dimensionColumn: 'testname', rows: rows1 },
        { dimensionColumn: 'buildbranch', rows: rows2 },
      ],
    };
    render(<BreakdownTableWidget data={data} />);

    // Default is 10 rows per page. Check page 1.
    expect(screen.getByText('test0')).toBeInTheDocument();
    expect(screen.queryByText('test14')).not.toBeInTheDocument();

    // Click 'Go to next page' button
    const nextPageBtn = screen.getByRole('button', {
      name: /Go to next page/i,
    });
    fireEvent.click(nextPageBtn);
    expect(screen.getByText('test14')).toBeInTheDocument();
    expect(screen.queryByText('test0')).not.toBeInTheDocument();

    // Switch dimension tab
    const categorySelect = screen.getByRole('combobox', {
      name: 'Breakdown by category',
    });
    fireEvent.mouseDown(categorySelect);
    const listbox = screen.getByRole('listbox');
    fireEvent.click(listbox.childNodes[1]);

    // Formatting check: We switched to BUILDBRANCH.
    // State is hoisted and explicitly resets pageIndex to 0 to avoid empty tables.
    expect(screen.getByText('branch0')).toBeInTheDocument();
    expect(screen.queryByText('branch14')).not.toBeInTheDocument();
  });

  it('handles missing dimensionColumn and missing rows gracefully', () => {
    // Branch coverage: section.dimensionColumn || 'unknown' and empty rows array
    const data = {
      sections: [
        {
          dimensionColumn: '',
          rows: [],
        },
      ],
    };
    render(<BreakdownTableWidget data={data} />);
    screen.debug(
      screen.getByRole('combobox', { name: 'Breakdown by category' }),
    );
    expect(
      screen.getByRole('combobox', { name: 'Breakdown by category' }),
    ).toHaveTextContent(/UNKNOWN/i);
    // As there are no rows, there should be no data
    expect(screen.getByText('No records to display')).toBeInTheDocument();
  });

  it('handles null/undefined values in DimensionCell and MetricCell', () => {
    const data = {
      sections: [
        {
          dimensionColumn: 'testname',
          rows: [
            // Dimension is undefined, Metric is null, Custom is a string
            {
              dimension_value: undefined,
              COUNT: null,
              CUSTOM: 'hello',
            },
          ],
        },
      ],
    };
    render(<BreakdownTableWidget data={data} />);
    // DimensionCell falls back to empty string when undefined
    const cells = screen.getAllByRole('cell');
    // First cell (dimension) should be empty (though MUI Chip renders it)
    expect(cells[0]).toHaveTextContent('');
    // COUNT column (metric) should be empty for null
    expect(cells[1]).toHaveTextContent('');
    // CUSTOM column should be original string for non-number
    expect(cells[2]).toHaveTextContent('hello');
  });
});
