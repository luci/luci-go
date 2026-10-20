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

import { act, fireEvent, render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';

import { colors } from '@/fleet/theme/colors';

import { AnnotationMarker } from './annotation_marker';
import {
  getAnnotationTimestampMs,
  TrendlineAnnotation,
} from './trendline_annotations';

describe('AnnotationMarker', () => {
  const sampleAnnotation: TrendlineAnnotation = {
    timestamp: DateTime.fromISO('2026-10-20T00:00:00Z', { zone: 'utc' }),
    title: 'Algorithm update',
    description: 'Updated health calculations for extra statuses.',
  };

  const renderMarker = (props = {}) => {
    return render(
      <svg width={800} height={400}>
        <AnnotationMarker
          x1={200}
          y1={350}
          x2={200}
          y2={50}
          annotation={sampleAnnotation}
          {...props}
        />
      </svg>,
    );
  };

  it('renders null when coordinates are missing or invalid', () => {
    const { container } = render(
      <svg width={800} height={400}>
        <AnnotationMarker
          x1={undefined}
          y1={350}
          x2={200}
          y2={50}
          annotation={sampleAnnotation}
        />
      </svg>,
    );
    expect(container.querySelector('line')).toBeNull();
  });

  it('renders null when coordinates are not finite', () => {
    const { container } = render(
      <svg width={800} height={400}>
        <AnnotationMarker
          x1={NaN}
          y1={350}
          x2={200}
          y2={50}
          annotation={sampleAnnotation}
        />
      </svg>,
    );
    expect(container.querySelector('line')).toBeNull();
  });

  it('renders the vertical dashed line and hit area with correct geometry', () => {
    renderMarker();

    const line = screen.getByTestId('trendline-annotation-line');
    expect(line).toBeInTheDocument();
    expect(line).toHaveAttribute('x1', '200');
    expect(line).toHaveAttribute('y1', '350');
    expect(line).toHaveAttribute('x2', '200');
    expect(line).toHaveAttribute('y2', '50');
    expect(line).toHaveAttribute('stroke', colors.grey[400]);
    expect(line).toHaveAttribute('stroke-dasharray', '4 4');

    const hitArea = screen.getByTestId('trendline-annotation-hit-area');
    expect(hitArea).toBeInTheDocument();
    expect(hitArea).toHaveAttribute('stroke', 'transparent');
    expect(hitArea).toHaveAttribute('stroke-width', '16');
  });

  it('has outline: none on the interactive <g> element to prevent browser click bounding box', () => {
    renderMarker();

    const ts = getAnnotationTimestampMs(sampleAnnotation);
    const markerGroup = screen.getByTestId(`trendline-annotation-${ts}`);
    expect(markerGroup).toHaveStyle({ outline: 'none' });
    expect(markerGroup).toHaveAttribute('tabindex', '0');
  });

  it('transitions to active colors on mouse hover', () => {
    renderMarker();

    const ts = getAnnotationTimestampMs(sampleAnnotation);
    const markerGroup = screen.getByTestId(`trendline-annotation-${ts}`);
    const line = screen.getByTestId('trendline-annotation-line');
    const badge = screen.getByTestId('trendline-annotation-badge');

    expect(line).toHaveAttribute('stroke', colors.grey[400]);
    expect(badge).toHaveAttribute('fill', colors.white);

    act(() => {
      fireEvent.mouseEnter(markerGroup);
    });

    expect(line).toHaveAttribute('stroke', colors.grey[500]);
    expect(badge).toHaveAttribute('fill', colors.grey[500]);

    act(() => {
      fireEvent.mouseLeave(markerGroup);
    });

    expect(line).toHaveAttribute('stroke', colors.grey[400]);
    expect(badge).toHaveAttribute('fill', colors.white);
  });

  it('renders circular focus ring when keyboard focused', () => {
    const { container } = renderMarker();

    const ts = getAnnotationTimestampMs(sampleAnnotation);
    const markerGroup = screen.getByTestId(`trendline-annotation-${ts}`);

    // No focus ring initially
    let circles = container.querySelectorAll('circle');
    expect(circles.length).toBe(1);

    act(() => {
      fireEvent.focus(markerGroup);
    });

    // Circular focus ring appears
    circles = container.querySelectorAll('circle');
    expect(circles.length).toBe(2);
    const focusRing = circles[0];
    expect(focusRing).toHaveAttribute('stroke', colors.grey[500]);
    expect(focusRing).toHaveAttribute('fill', 'none');

    act(() => {
      fireEvent.blur(markerGroup);
    });

    circles = container.querySelectorAll('circle');
    expect(circles.length).toBe(1);
  });
});
