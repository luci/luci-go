// Copyright 2023 The LUCI Authors.
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
import { act } from 'react';

import { RelativeDurationBadge } from './relative_duration_badge';

describe('RelativeDurationBadge', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  test('duration should update when `to` is not specified', async () => {
    render(
      <RelativeDurationBadge from={DateTime.now().minus({ minutes: 10 })} />,
    );

    expect(screen.getByTestId('duration')).toHaveTextContent('10m');

    await act(() => jest.advanceTimersByTimeAsync(600000));
    expect(screen.getByTestId('duration')).toHaveTextContent('20m');

    await act(() => jest.advanceTimersByTimeAsync(300000));
    expect(screen.getByTestId('duration')).toHaveTextContent('25m');
  });

  test('duration should NOT update when `to` is specified', async () => {
    render(
      <RelativeDurationBadge
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
});
