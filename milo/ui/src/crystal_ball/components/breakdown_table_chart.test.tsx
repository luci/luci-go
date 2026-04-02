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

import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { BreakdownSection } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { BreakdownTableChart } from './breakdown_table_chart';

describe('BreakdownTableChart', () => {
  const mockOnUpdateAggregations = jest.fn();

  it(`renders "${COMMON_MESSAGES.NO_DATA_FOUND}" when sections are empty`, () => {
    render(
      <BreakdownTableChart
        sections={[]}
        currentAggregations={[]}
        onUpdateAggregations={mockOnUpdateAggregations}
        hasSeries={true}
      />,
    );
    expect(screen.getByText(COMMON_MESSAGES.NO_DATA_FOUND)).toBeInTheDocument();
  });

  it('renders the tabs and table when data is provided', () => {
    const sections: BreakdownSection[] = [
      {
        dimensionColumn: 'testname',
        rows: [{ dimension_value: 'test1', COUNT: 10, MIN: 1, MAX: 5 }],
      },
    ];

    render(
      <BreakdownTableChart
        sections={sections}
        currentAggregations={[]}
        onUpdateAggregations={mockOnUpdateAggregations}
        hasSeries={true}
      />,
    );

    expect(
      screen.getByRole('combobox', { name: 'Breakdown by category' }),
    ).toHaveTextContent(/TESTNAME/i);
    expect(screen.getByText('test1')).toBeInTheDocument();
  });

  it('switches categories and displays corresponding data', () => {
    const sections: BreakdownSection[] = [
      {
        dimensionColumn: 'testname',
        rows: [{ dimension_value: 'test1', COUNT: 10, MIN: 1, MAX: 5 }],
      },
      {
        dimensionColumn: 'buildbranch',
        rows: [{ dimension_value: 'branch1', COUNT: 20, MIN: 2, MAX: 6 }],
      },
    ];

    render(
      <BreakdownTableChart
        sections={sections}
        currentAggregations={[]}
        onUpdateAggregations={mockOnUpdateAggregations}
        hasSeries={true}
      />,
    );

    expect(
      screen.getByRole('combobox', { name: 'Breakdown by category' }),
    ).toHaveTextContent(/TESTNAME/i);
    expect(screen.getByText('test1')).toBeInTheDocument();

    const select = screen.getByRole('combobox', {
      name: 'Breakdown by category',
    });
    fireEvent.mouseDown(select);

    const listbox = screen.getByRole('listbox');
    fireEvent.click(listbox.childNodes[1]);

    expect(
      screen.getByRole('combobox', { name: 'Breakdown by category' }),
    ).toHaveTextContent(/BUILDBRANCH/i);
    expect(screen.getByText('branch1')).toBeInTheDocument();
  });

  it('dynamically derives metric columns from data', () => {
    const sections: BreakdownSection[] = [
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
    ];

    render(
      <BreakdownTableChart
        sections={sections}
        currentAggregations={[]}
        onUpdateAggregations={mockOnUpdateAggregations}
        hasSeries={true}
      />,
    );

    expect(screen.getByText('PASSES')).toBeInTheDocument();
    expect(screen.getByText('FAILURES')).toBeInTheDocument();
    expect(screen.getByText('CUSTOMSCORE')).toBeInTheDocument();
  });

  it('resets pagination state to page 0 across category switches', () => {
    const rows1 = Array.from({ length: 15 }, (_, i) => ({
      dimension_value: `test${i}`,
      COUNT: 100 - i,
    }));
    const rows2 = Array.from({ length: 15 }, (_, i) => ({
      dimension_value: `branch${i}`,
      COUNT: 200 - i,
    }));

    const sections: BreakdownSection[] = [
      { dimensionColumn: 'testname', rows: rows1 },
      { dimensionColumn: 'buildbranch', rows: rows2 },
    ];

    render(
      <BreakdownTableChart
        sections={sections}
        currentAggregations={[]}
        onUpdateAggregations={mockOnUpdateAggregations}
        hasSeries={true}
      />,
    );

    expect(screen.getByText('test0')).toBeInTheDocument();

    const nextPageBtn = screen.getByRole('button', {
      name: /Go to next page/i,
    });
    fireEvent.click(nextPageBtn);
    expect(screen.getByText('test14')).toBeInTheDocument();

    const categorySelect = screen.getByRole('combobox', {
      name: 'Breakdown by category',
    });
    fireEvent.mouseDown(categorySelect);
    const listbox = screen.getByRole('listbox');
    fireEvent.click(listbox.childNodes[1]);

    expect(screen.getByText('branch0')).toBeInTheDocument();
    expect(screen.queryByText('branch14')).not.toBeInTheDocument();
  });

  it('handles missing dimensionColumn and missing rows gracefully', () => {
    const sections: BreakdownSection[] = [
      {
        dimensionColumn: '',
        rows: [],
      },
    ];

    render(
      <BreakdownTableChart
        sections={sections}
        currentAggregations={[]}
        onUpdateAggregations={mockOnUpdateAggregations}
        hasSeries={true}
      />,
    );

    expect(
      screen.getByRole('combobox', { name: 'Breakdown by category' }),
    ).toHaveTextContent(/UNKNOWN/i);
    expect(screen.getByText(COMMON_MESSAGES.NO_DATA_FOUND)).toBeInTheDocument();
  });

  it('handles null/undefined values in cells', () => {
    const sections: BreakdownSection[] = [
      {
        dimensionColumn: 'testname',
        rows: [
          {
            dimension_value: undefined,
            COUNT: null,
            CUSTOM: 'invalid_number',
          },
        ],
      },
    ];

    render(
      <BreakdownTableChart
        sections={sections}
        currentAggregations={[]}
        onUpdateAggregations={mockOnUpdateAggregations}
        hasSeries={true}
      />,
    );

    const cells = screen.getAllByRole('cell');
    expect(cells[0]).toHaveTextContent('-');
    expect(cells[1]).toHaveTextContent('-');
    expect(cells[2]).toHaveTextContent('-');
    expect(cells[3]).toHaveTextContent('invalid_number');
  });

  it('allows opening the aggregates select dropdown', () => {
    const sections: BreakdownSection[] = [
      {
        dimensionColumn: 'testname',
        rows: [{ dimension_value: 'test1', COUNT: 10, MIN: 1, MAX: 5 }],
      },
    ];

    render(
      <BreakdownTableChart
        sections={sections}
        currentAggregations={[]}
        onUpdateAggregations={mockOnUpdateAggregations}
        hasSeries={true}
      />,
    );

    const aggregatesSelect = screen.getByRole('combobox', {
      name: 'Aggregates',
    });
    fireEvent.mouseDown(aggregatesSelect);

    const listbox = screen.getByRole('listbox');
    expect(listbox).toBeInTheDocument();
  });
});
