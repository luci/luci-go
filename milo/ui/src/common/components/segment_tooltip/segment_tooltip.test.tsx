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

import {
  Segment,
  Segment_Counts,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { SegmentTooltip } from './segment_tooltip';

describe('<SegmentTooltip />', () => {
  const baseSegment = Segment.fromPartial({
    startPosition: '100',
    endPosition: '110',
    startHour: '2026-07-24T12:00:00Z',
    endHour: '2026-07-24T14:00:00Z',
    counts: Segment_Counts.fromPartial({
      unexpectedResults: 10,
      totalResults: 100,
      unexpectedVerdicts: 5,
      flakyVerdicts: 5,
      totalVerdicts: 50,
    }),
  });

  it('renders commit range and total commit count', () => {
    render(<SegmentTooltip segment={baseSegment} />);

    expect(screen.getByText(/Start:\s*100/)).toBeInTheDocument();
    expect(screen.getByText(/End:\s*110/)).toBeInTheDocument();
    expect(screen.getByText(/11\s*commits/)).toBeInTheDocument();
  });

  it('renders blamelist links when blamelistBaseUrl is provided', () => {
    render(
      <SegmentTooltip
        segment={baseSegment}
        blamelistBaseUrl="/ui/labs/p/chromium/tests/test_id/variants/hash/refs/ref/blamelist"
      />,
    );

    const startLink = screen.getByRole('link', { name: '100' });
    expect(startLink).toHaveAttribute(
      'href',
      '/ui/labs/p/chromium/tests/test_id/variants/hash/refs/ref/blamelist?expand=CP-100#CP-100',
    );

    const endLink = screen.getByRole('link', { name: '110' });
    expect(endLink).toHaveAttribute(
      'href',
      '/ui/labs/p/chromium/tests/test_id/variants/hash/refs/ref/blamelist?expand=CP-110#CP-110',
    );
  });

  it('renders before and after retries breakdown when counts exist', () => {
    render(<SegmentTooltip segment={baseSegment} />);

    expect(screen.getByText('Before Retries')).toBeInTheDocument();
    expect(
      screen.getByText('10% of 100 test results failed'),
    ).toBeInTheDocument();

    expect(screen.getByText('After Retries')).toBeInTheDocument();
    expect(screen.getByText('failed')).toBeInTheDocument();
    expect(screen.getByText('flaky')).toBeInTheDocument();
    expect(screen.getByText('passed')).toBeInTheDocument();
    expect(screen.getByText('of 50 test verdicts')).toBeInTheDocument();
  });

  it('does not render breakdown boxes when counts are missing', () => {
    const segmentNoCounts = Segment.fromPartial({
      startPosition: '100',
      endPosition: '110',
    });

    render(<SegmentTooltip segment={segmentNoCounts} />);

    expect(screen.queryByText('Before Retries')).not.toBeInTheDocument();
    expect(screen.queryByText('After Retries')).not.toBeInTheDocument();
  });

  it('renders invocation segment context message when specified', () => {
    render(
      <SegmentTooltip segment={baseSegment} segmentContextType="invocation" />,
    );

    expect(
      screen.getByText('This segment contains the current test result'),
    ).toBeInTheDocument();
  });

  it('renders beforeInvocation segment context message when specified', () => {
    render(
      <SegmentTooltip
        segment={baseSegment}
        segmentContextType="beforeInvocation"
      />,
    );

    expect(
      screen.getByText('This segment is older than the current test result'),
    ).toBeInTheDocument();
  });

  it('renders afterInvocation segment context message when specified', () => {
    render(
      <SegmentTooltip
        segment={baseSegment}
        segmentContextType="afterInvocation"
      />,
    );

    expect(
      screen.getByText('This segment is newer than the current test result'),
    ).toBeInTheDocument();
  });
});
