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
import { fireEvent, render, screen } from '@testing-library/react';

import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { RawSampleRow } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { RawSampleList } from './raw_sample_list';

describe('RawSampleList', () => {
  const currentYear = new Date().getFullYear();
  const mockRows: RawSampleRow[] = [
    {
      values: {
        value: '100',
        test_name: 'test_class#test_method',
        atp_test_name: 'atp_test',
        build_id: '123',
        build_branch: 'main',
        build_target: 'target',
        invocation_complete_timestamp: `${currentYear}-04-04T10:00:00Z`,
        other_field: 'other_value',
      },
    },
  ];

  const mockExpandedItems = new Set<number>();
  const mockOnToggleExpand = jest.fn();

  it('should render "No raw samples found" when rows are empty', () => {
    render(
      <RawSampleList
        rows={[]}
        expandedItems={mockExpandedItems}
        onToggleExpand={mockOnToggleExpand}
      />,
    );
    expect(
      screen.getByText(COMMON_MESSAGES.NO_RAW_SAMPLES_FOUND),
    ).toBeInTheDocument();
  });

  it('should render a list of items when rows are provided', () => {
    render(
      <RawSampleList
        rows={mockRows}
        expandedItems={mockExpandedItems}
        onToggleExpand={mockOnToggleExpand}
      />,
    );
    expect(screen.getAllByText('100')).not.toHaveLength(0);
    expect(screen.getAllByText('atp_test')).not.toHaveLength(0);
    expect(screen.getAllByText(/test_class/)).not.toHaveLength(0);
    expect(screen.getAllByText(/#test_method/)).not.toHaveLength(0);
    expect(screen.getAllByText('123')).not.toHaveLength(0);
    expect(screen.getAllByText('main')).not.toHaveLength(0);
    expect(screen.getAllByText('target')).not.toHaveLength(0);
  });

  it('should expand accordion to show additional details', () => {
    render(
      <RawSampleList
        rows={mockRows}
        expandedItems={mockExpandedItems}
        onToggleExpand={mockOnToggleExpand}
      />,
    );
    const accordion = screen.getByText(COMMON_MESSAGES.ALL_DETAILS);
    fireEvent.click(accordion);

    expect(screen.getByText('Other Field:')).toBeInTheDocument();
    expect(screen.getByText('other_value')).toBeInTheDocument();
  });

  it('should format timestamp correctly', () => {
    render(
      <RawSampleList
        rows={mockRows}
        expandedItems={mockExpandedItems}
        onToggleExpand={mockOnToggleExpand}
      />,
    );
    const elements = screen.getAllByText(new RegExp(currentYear.toString()));
    expect(elements.length).toBeGreaterThanOrEqual(1);
  });

  it('should apply wordBreak: "break-all" to test name', () => {
    render(
      <RawSampleList
        rows={mockRows}
        expandedItems={mockExpandedItems}
        onToggleExpand={mockOnToggleExpand}
      />,
    );
    const elements = screen.getAllByText(/test_class/);
    expect(elements.length).toBeGreaterThanOrEqual(1);
    expect(elements[0]).toHaveStyle({ wordBreak: 'break-all' });
  });
});
