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

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';
import { act } from 'react';

import { Timestamp } from './timestamp';

describe('<Timestamp />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.setSystemTime(DateTime.fromISO('2020-03-03T01:01:01.11').toMillis());
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
  });

  it('should display timestamp', () => {
    render(
      <Timestamp
        datetime={DateTime.fromISO('2020-02-02T02:02:02.22')}
        format="yyyy MM dd hh:mm"
      />,
    );

    expect(screen.getByText('2020 02 02 02:02')).toBeVisible();
  });

  it('should display tooltip on hover', async () => {
    render(
      <Timestamp
        datetime={DateTime.fromISO('2020-02-02T02:02:02.22')}
        extraTimezones={{
          zones: [
            { label: 'first time zone', zone: 'UTC' },
            { label: 'second time zone', zone: 'UTC+7' },
          ],
          format: 'yyyy MM dd hh:mm',
        }}
      />,
    );

    fireEvent.mouseEnter(screen.getByText('02:02:02 Sun, Feb 02 2020 UTC'));
    await act(() => jest.runAllTimersAsync());

    expect(screen.getByText('29 days 22 hours ago')).toBeVisible();
    expect(
      screen.getByText('first time zone:').nextElementSibling,
    ).toHaveTextContent('2020 02 02 02:02');
    expect(
      screen.getByText('second time zone:').nextElementSibling,
    ).toHaveTextContent('2020 02 02 09:02');
  });
});
