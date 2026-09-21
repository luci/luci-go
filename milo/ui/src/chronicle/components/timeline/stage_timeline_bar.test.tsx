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

import { fireEvent, render } from '@testing-library/react';
import { DateTime } from 'luxon';

import { StageResultStatus } from '@/chronicle/utils/check_utils';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';

import { ProgressSegment } from './segments';
import { StageTimelineBar } from './stage_timeline_bar';
import { SELECTED_BAR_STYLE, TimelineItem } from './types';

describe('StageTimelineBar', () => {
  const start = DateTime.fromISO('2026-09-15T00:00:00Z');
  const end = DateTime.fromISO('2026-09-15T01:00:00Z');
  // Simple linear scale mapping 1 hour to 600px (10px per minute)
  const xScale = (d: DateTime) => d.diff(start, 'minutes').minutes * 10;

  const baseItem: TimelineItem = {
    id: 'stage-1',
    label: 'Stage 1',
    start,
    end,
    stage: Stage.fromPartial({ identifier: { id: 'stage-1' } }),
    resultStatus: StageResultStatus.SUCCESS,
  };

  it('renders single fallback rect when segments is empty', () => {
    const onClick = jest.fn();
    const { container } = render(
      <svg>
        <StageTimelineBar
          item={baseItem}
          xScale={xScale}
          isSelected={false}
          onClick={onClick}
        />
      </svg>,
    );

    const rects = container.querySelectorAll('rect');
    expect(rects).toHaveLength(1);
    expect(rects[0]).toHaveAttribute('x', '0');
    expect(rects[0]).toHaveAttribute('width', '600');

    fireEvent.click(rects[0]);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it('renders segmented rects, clip path, and titles when segments are present', () => {
    const segments: ProgressSegment[] = [
      {
        start: start.plus({ minutes: 0 }),
        end: start.plus({ minutes: 10 }),
        label: 'fetch',
        rawMessage: 'Fetching sources',
        attemptNumber: 1,
      },
      {
        start: start.plus({ minutes: 10 }),
        end: start.plus({ minutes: 30 }),
        label: 'compile',
        rawMessage: 'Compiling code',
        attemptNumber: 1,
      },
      {
        start: start.plus({ minutes: 30 }),
        end: start.plus({ minutes: 60 }),
        label: 'test',
        rawMessage: 'Running tests',
        attemptNumber: 1,
      },
    ];

    const item: TimelineItem = {
      ...baseItem,
      segments,
    };

    const { container } = render(
      <svg>
        <StageTimelineBar
          item={item}
          xScale={xScale}
          isSelected={false}
          onClick={jest.fn()}
        />
      </svg>,
    );

    // Should have clipPath with rect, 3 segment rects, and 1 border overlay rect = 5 rects total
    const rects = container.querySelectorAll('rect');
    expect(rects.length).toBe(5);

    // Clip path
    const clipPath = container.querySelector('#clip-stage-stage-1-att-1');
    expect(clipPath).toBeInTheDocument();

    // Tooltips
    const titles = Array.from(container.querySelectorAll('title')).map(
      (t) => t.textContent,
    );
    expect(titles).toEqual([
      'Attempt 1: Fetching sources (10 min)',
      'Attempt 1: Compiling code (20 min)',
      'Attempt 1: Running tests (30 min)',
    ]);

    // Check uniform color: no fill-opacity on segments
    const segmentRects = container.querySelectorAll('g[clip-path] rect');
    expect(segmentRects).toHaveLength(3);
    for (const rect of segmentRects) {
      expect(rect).not.toHaveAttribute('fill-opacity');
    }

    // Divider lines between segments (idx > 0)
    const lines = container.querySelectorAll('line');
    expect(lines).toHaveLength(2);
    expect(lines[0]).toHaveAttribute('stroke', 'rgba(0, 0, 0, 0.4)');
  });

  it('renders each attempt as a separate bar without border lines in the gap', () => {
    const segments: ProgressSegment[] = [
      {
        start: start.plus({ minutes: 0 }),
        end: start.plus({ minutes: 10 }),
        label: 'first try',
        rawMessage: 'first try',
        attemptNumber: 1,
      },
      // 10-minute gap between minute 10 and minute 20
      {
        start: start.plus({ minutes: 20 }),
        end: start.plus({ minutes: 40 }),
        label: 'second try',
        rawMessage: 'second try',
        attemptNumber: 2,
      },
    ];

    const item: TimelineItem = {
      ...baseItem,
      segments,
    };

    const { container } = render(
      <svg>
        <StageTimelineBar
          item={item}
          xScale={xScale}
          isSelected={false}
          onClick={jest.fn()}
        />
      </svg>,
    );

    // Each attempt has its own border rect: Attempt 1 at x=0, Attempt 2 at x=200
    const borderRects = container.querySelectorAll('rect[fill="none"]');
    expect(borderRects).toHaveLength(2);
    expect(borderRects[0]).toHaveAttribute('x', '0');
    expect(borderRects[0]).toHaveAttribute('width', '100');
    expect(borderRects[1]).toHaveAttribute('x', '200');
    expect(borderRects[1]).toHaveAttribute('width', '200');

    // Both attempts share uniform background fill
    const segmentRects = container.querySelectorAll('g[clip-path] rect');
    expect(segmentRects[0]).toHaveAttribute('fill', 'var(--success-bg-color)');
    expect(segmentRects[1]).toHaveAttribute('fill', 'var(--success-bg-color)');

    // No divider lines in the gap between attempts
    const lines = container.querySelectorAll('line');
    expect(lines).toHaveLength(0);
  });

  it('hides segment label when segment width is below SEGMENT_LABEL_MIN_WIDTH_PX', () => {
    // 2 minutes * 10px/min = 20px, which is < SEGMENT_LABEL_MIN_WIDTH_PX (28px)
    const segments: ProgressSegment[] = [
      {
        start: start.plus({ minutes: 0 }),
        end: start.plus({ minutes: 2 }),
        label: 'tiny',
        rawMessage: 'Tiny step',
        attemptNumber: 1,
      },
      {
        start: start.plus({ minutes: 2 }),
        end: start.plus({ minutes: 30 }),
        label: 'wide',
        rawMessage: 'Wide step',
        attemptNumber: 1,
      },
    ];

    const item: TimelineItem = {
      ...baseItem,
      segments,
    };

    const { container } = render(
      <svg>
        <StageTimelineBar
          item={item}
          xScale={xScale}
          isSelected={false}
          onClick={jest.fn()}
        />
      </svg>,
    );

    const textElements = Array.from(container.querySelectorAll('text')).map(
      (t) => t.textContent,
    );
    expect(textElements).not.toContain('tiny');
    expect(textElements).toContain('wide');
  });

  it('applies selected styles when isSelected is true', () => {
    const item: TimelineItem = {
      ...baseItem,
      segments: [
        {
          start,
          end,
          label: 'phase',
          rawMessage: 'phase',
          attemptNumber: 1,
        },
      ],
    };

    const { container } = render(
      <svg>
        <StageTimelineBar
          item={item}
          xScale={xScale}
          isSelected={true}
          onClick={jest.fn()}
        />
      </svg>,
    );

    const segmentRect = container.querySelector('g[clip-path] rect');
    expect(segmentRect).toHaveAttribute('fill', SELECTED_BAR_STYLE.fill);

    const overlayRect = container.querySelector('rect[fill="none"]');
    expect(overlayRect).toHaveAttribute('stroke', SELECTED_BAR_STYLE.stroke);

    const text = container.querySelector('text');
    expect(text).toHaveAttribute('fill', 'var(--default-text-color)');
  });
});
