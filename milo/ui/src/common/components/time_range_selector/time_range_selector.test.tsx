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

import { fireEvent, render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { URLObserver } from '@/testing_tools/url_observer';

import { TimeRangeSelector } from './time_range_selector';
import { CUSTOMIZE_OPTION } from './time_range_selector_utils';

describe('<TimeRangeSelector />', () => {
  afterEach(() => {
    jest.restoreAllMocks(); // Clean up mocks after each test
  });
  it('use default when no search param', () => {
    render(
      <FakeContextProvider>
        <TimeRangeSelector />
      </FakeContextProvider>,
    );
    const buttonElement = screen.getByTestId('time-button');
    expect(buttonElement).toHaveTextContent('Last 3 days');
  });

  it('can set time range selection data via search param: absolute time range', () => {
    render(
      <FakeContextProvider
        mountedPath="/"
        routerOptions={{
          initialEntries: [
            '?time_option=customize&' +
              'start_time=1721916000&' +
              'end_time=1722434400',
          ],
        }}
      >
        <TimeRangeSelector />
      </FakeContextProvider>,
    );
    const buttonElement = screen.getByTestId('time-button');
    expect(buttonElement).toHaveTextContent(
      '2024-07-25 14:00 - 2024-07-31 14:00',
    );
  });
  it('can set time range selection data via search param: relative time range', () => {
    render(
      <FakeContextProvider
        mountedPath="/"
        routerOptions={{
          initialEntries: ['?time_option=7d'],
        }}
      >
        <TimeRangeSelector />
      </FakeContextProvider>,
    );
    const buttonElement = screen.getByTestId('time-button');
    expect(buttonElement).toHaveTextContent('Last 7 days');
  });

  it('can save time range selection data to search param', () => {
    const urlCallback = jest.fn();
    render(
      <FakeContextProvider>
        <TimeRangeSelector />
        <URLObserver callback={urlCallback} />
      </FakeContextProvider>,
    );

    // Select customise.
    const buttonElement = screen.getByTestId('time-button');
    fireEvent.click(buttonElement);
    const customiseMenuItem = screen.getByTestId(CUSTOMIZE_OPTION);
    fireEvent.click(customiseMenuItem);
    const fromTimeInputEle = screen.getByLabelText('From (UTC)');
    fireEvent.change(fromTimeInputEle, {
      target: { value: '06/10/2024 12:00 AM' },
    });
    const toTimeInputEle = screen.getByLabelText('To (UTC)');
    fireEvent.change(toTimeInputEle, {
      target: { value: '06/12/2024 11:00 PM' },
    });
    expect(urlCallback).toHaveBeenLastCalledWith(
      expect.objectContaining({
        search: {
          time_option: CUSTOMIZE_OPTION,
          start_time: DateTime.fromFormat('06/10/2024 12:00 AM', 'MM/dd/yyyy t')
            .toUTC()
            .toUnixInteger()
            .toString(),
          end_time: DateTime.fromFormat('06/12/2024 11:00 PM', 'MM/dd/yyyy t')
            .toUTC()
            .toUnixInteger()
            .toString(),
        },
      }),
    );

    // Select Last 7 days.
    fireEvent.click(buttonElement);
    const last7dMenuItem = screen.getByTestId('7d');
    fireEvent.click(last7dMenuItem);
    expect(urlCallback).toHaveBeenLastCalledWith(
      expect.objectContaining({
        search: {
          time_option: '7d',
        },
      }),
    );
  });

  it('can prefill customized time range with previous relative time range selection', () => {
    const urlCallback = jest.fn();
    render(
      <FakeContextProvider>
        <TimeRangeSelector />
        <URLObserver callback={urlCallback} />
      </FakeContextProvider>,
    );

    const mockNow = DateTime.fromISO('2024-08-15T12:00:00.000Z');
    jest.spyOn(DateTime, 'now').mockReturnValue(mockNow);

    // Select Last 7 days.
    const buttonElement = screen.getByTestId('time-button');
    fireEvent.click(buttonElement);
    const last7dMenuItem = screen.getByTestId('7d');
    fireEvent.click(last7dMenuItem);
    expect(urlCallback).toHaveBeenLastCalledWith(
      expect.objectContaining({
        search: {
          time_option: '7d',
        },
      }),
    );

    // Select customise.
    const customiseMenuItem = screen.getByTestId(CUSTOMIZE_OPTION);
    fireEvent.click(customiseMenuItem);
    expect(urlCallback).toHaveBeenLastCalledWith(
      expect.objectContaining({
        search: {
          time_option: CUSTOMIZE_OPTION,
          start_time: DateTime.fromFormat('08/08/2024 12:00 PM', 'MM/dd/yyyy t')
            .toUTC()
            .toUnixInteger()
            .toString(),
          end_time: DateTime.fromFormat('08/15/2024 12:00 PM', 'MM/dd/yyyy t')
            .toUTC()
            .toUnixInteger()
            .toString(),
        },
      }),
    );
  });
});
