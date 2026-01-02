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

import { render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';

import { Segment } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { TestAddedInfo } from './test_added_info';

describe('<TestAddedInfo />', () => {
  it('should render with timestamp and blamelist link with context', () => {
    const mockSegment = Segment.fromPartial({
      startPosition: '100',
      startHour: '2024-01-01T10:00:00Z',
    });
    const mockNow = DateTime.fromISO('2024-01-01T12:00:00Z');
    const mockBlamelistBaseUrl = '/blamelist/base/url';

    render(
      <TestAddedInfo
        segment={mockSegment}
        nowDtForFormatting={mockNow}
        blamelistBaseUrl={mockBlamelistBaseUrl}
      />,
    );

    expect(screen.getByText(/Test added/)).toBeInTheDocument();

    const blamelistLink = screen.getByRole('link', { name: 'blamelist' });
    expect(blamelistLink).toBeInTheDocument();
    // Should have start_cp=CP-100 and end_cp=CP-70 (100 - 30)
    expect(blamelistLink).toHaveAttribute(
      'href',
      '/blamelist/base/url?start_cp=CP-100&end_cp=CP-70#CP-100',
    );
  });

  it('should clamp end_cp to 0', () => {
    const mockSegment = Segment.fromPartial({
      startPosition: '10',
      startHour: '2024-01-01T10:00:00Z',
    });
    const mockNow = DateTime.fromISO('2024-01-01T12:00:00Z');
    const mockBlamelistBaseUrl = '/blamelist/base/url';

    render(
      <TestAddedInfo
        segment={mockSegment}
        nowDtForFormatting={mockNow}
        blamelistBaseUrl={mockBlamelistBaseUrl}
      />,
    );

    const blamelistLink = screen.getByRole('link', { name: 'blamelist' });
    expect(blamelistLink).toBeInTheDocument();
    // Should have start_cp=CP-10 and end_cp=CP-0 (10 - 30 -> 0)
    expect(blamelistLink).toHaveAttribute(
      'href',
      '/blamelist/base/url?start_cp=CP-10&end_cp=CP-0#CP-10',
    );
  });
});
