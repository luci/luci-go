// Copyright 2024 The LUCI Authors.
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
import { scaleThreshold } from 'd3';
import { DateTime, Duration } from 'luxon';
import { act } from 'react';

import { DurationBadge } from './duration_badge';

const testColorScaleMs = scaleThreshold(
  [
    Duration.fromObject({ seconds: 20 }).toMillis(),
    Duration.fromObject({ minutes: 1 }).toMillis(),
    Duration.fromObject({ minutes: 5 }).toMillis(),
    Duration.fromObject({ minutes: 15 }).toMillis(),
    Duration.fromObject({ hours: 1 }).toMillis(),
    Duration.fromObject({ hours: 3 }).toMillis(),
    Duration.fromObject({ hours: 12 }).toMillis(),
  ],
  [
    {
      backgroundColor: 'hsl(206, 85%, 95%)',
      color: 'var(--light-text-color)',
    },
    {
      backgroundColor: 'hsl(206, 85%, 85%)',
      color: 'var(--light-text-color)',
    },
    { backgroundColor: 'hsl(206, 85%, 75%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 65%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 55%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 45%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 35%)', color: 'white' },
    { backgroundColor: 'hsl(206, 85%, 25%)', color: 'white' },
  ],
);

const testColorScale = (d: Duration) => testColorScaleMs(d.toMillis());

describe('<DurationBadge />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('duration should update when only `from` is specified', async () => {
    render(<DurationBadge from={DateTime.now().minus({ minutes: 10 })} />);

    expect(screen.getByTestId('duration')).toHaveTextContent('10m');

    await act(() => jest.advanceTimersByTimeAsync(600000));
    expect(screen.getByTestId('duration')).toHaveTextContent('20m');

    await act(() => jest.advanceTimersByTimeAsync(300000));
    expect(screen.getByTestId('duration')).toHaveTextContent('25m');
  });

  it('duration should update when only `to` is specified', async () => {
    render(<DurationBadge to={DateTime.now().plus({ minutes: 25 })} />);

    expect(screen.getByTestId('duration')).toHaveTextContent('25m');

    await act(() => jest.advanceTimersByTimeAsync(600000));
    expect(screen.getByTestId('duration')).toHaveTextContent('15m');

    await act(() => jest.advanceTimersByTimeAsync(300000));
    expect(screen.getByTestId('duration')).toHaveTextContent('10m');
  });

  it('duration should NOT update when both `from` and `to` are specified', async () => {
    render(
      <DurationBadge
        from={DateTime.now().minus({ minutes: 10 })}
        to={DateTime.now().minus({ minutes: 5 })}
      />,
    );

    expect(screen.getByTestId('duration')).toHaveTextContent('5.0m');

    await act(() => jest.advanceTimersByTimeAsync(600000));
    expect(screen.getByTestId('duration')).toHaveTextContent('5.0m');

    await act(() => jest.advanceTimersByTimeAsync(300000));
    expect(screen.getByTestId('duration')).toHaveTextContent('5.0m');
  });

  it('duration should NOT update when `duration` is specified', async () => {
    render(
      <DurationBadge
        duration={Duration.fromObject({ minutes: 5 })}
        from={DateTime.now().minus({ minutes: 10 })}
      />,
    );

    expect(screen.getByTestId('duration')).toHaveTextContent('5.0m');

    await act(() => jest.advanceTimersByTimeAsync(600000));
    expect(screen.getByTestId('duration')).toHaveTextContent('5.0m');

    await act(() => jest.advanceTimersByTimeAsync(300000));
    expect(screen.getByTestId('duration')).toHaveTextContent('5.0m');
  });

  it('duration should NOT update when neither `from` nor `to` is specified', async () => {
    render(<DurationBadge />);

    expect(screen.getByTestId('duration')).toHaveTextContent('N/A');

    await act(() => jest.advanceTimersByTimeAsync(600000));
    expect(screen.getByTestId('duration')).toHaveTextContent('N/A');

    await act(() => jest.advanceTimersByTimeAsync(300000));
    expect(screen.getByTestId('duration')).toHaveTextContent('N/A');
  });

  it('color should scale with duration', async () => {
    const { rerender } = render(
      <DurationBadge
        duration={Duration.fromObject({ minutes: 5 })}
        colorScale={testColorScale}
      />,
    );
    expect(screen.getByTestId('duration')).toHaveStyleRule(
      'background-color',
      'hsl(206, 85%, 65%)',
    );

    rerender(
      <DurationBadge
        duration={Duration.fromObject({ hours: 2 })}
        colorScale={testColorScale}
      />,
    );
    expect(screen.getByTestId('duration')).toHaveStyleRule(
      'background-color',
      'hsl(206, 85%, 45%)',
    );
  });

  it('color should scale with duration when duration is not explicitly specified', async () => {
    render(
      <DurationBadge
        from={DateTime.now().minus({ hours: 2 })}
        colorScale={testColorScale}
      />,
    );
    expect(screen.getByTestId('duration')).toHaveStyleRule(
      'background-color',
      'hsl(206, 85%, 45%)',
    );

    await act(() => jest.advanceTimersByTimeAsync(3_600_000 * 1.5));
    expect(screen.getByTestId('duration')).toHaveStyleRule(
      'background-color',
      'hsl(206, 85%, 35%)',
    );
  });
});
