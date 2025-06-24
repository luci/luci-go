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

import { HistoryChangepoint } from './history_changepoint';

describe('<HistoryChangepoint />', () => {
  it('should render with timestamp and blamelist link', () => {
    const mockSegment = Segment.fromPartial({
      startPosition: '12345',
      startHour: '2024-01-01T10:00:00Z',
    });
    const mockNow = DateTime.fromISO('2024-01-01T12:00:00Z');
    const mockBlamelistBaseUrl = '/blamelist/base/url';

    render(
      <HistoryChangepoint
        pointingToSegment={mockSegment}
        nowDtForFormatting={mockNow}
        blamelistBaseUrl={mockBlamelistBaseUrl}
      />,
    );

    expect(screen.getByText('2 hours ago')).toBeInTheDocument();

    expect(screen.getByTestId('ArrowBackIcon')).toBeInTheDocument();

    const blamelistLink = screen.getByRole('link', { name: 'blamelist' });
    expect(blamelistLink).toBeInTheDocument();
    expect(blamelistLink).toHaveAttribute(
      'href',
      '/blamelist/base/url#CP-12345',
    );
  });

  it('should render without blamelist link when base url is not provided', () => {
    const mockSegment = Segment.fromPartial({
      startPosition: '12345',
      startHour: '2024-01-01T10:00:00Z',
    });
    const mockNow = DateTime.fromISO('2024-01-01T12:00:00Z');

    render(
      <HistoryChangepoint
        pointingToSegment={mockSegment}
        nowDtForFormatting={mockNow}
        blamelistBaseUrl={undefined}
      />,
    );

    expect(screen.getByText('2 hours ago')).toBeInTheDocument();
    expect(screen.getByTestId('ArrowBackIcon')).toBeInTheDocument();
    expect(
      screen.queryByRole('link', { name: 'blamelist' }),
    ).not.toBeInTheDocument();
  });

  it('should render without a timestamp when startHour is missing', () => {
    const mockSegment = Segment.fromPartial({
      startPosition: '12345',
    });
    const mockNow = DateTime.fromISO('2024-01-01T12:00:00Z');

    render(
      <HistoryChangepoint
        pointingToSegment={mockSegment}
        nowDtForFormatting={mockNow}
        blamelistBaseUrl="/blamelist/base/url"
      />,
    );

    expect(screen.queryByText(/ago/)).not.toBeInTheDocument();
    expect(screen.getByTestId('ArrowBackIcon')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'blamelist' })).toBeInTheDocument();
  });
});
