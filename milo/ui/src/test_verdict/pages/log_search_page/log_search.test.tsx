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

import { LogSearch } from './log_search';

describe('<LogSearch />', () => {
  it('can set form data via search param', () => {
    render(
      <FakeContextProvider
        mountedPath="/"
        routerOptions={{
          initialEntries: [
            '/?filter=%7B"startTime"%3A"2024-07-18T08%3A31%3A12.000Z"%2C ' +
              '"endTime"%3A"2024-07-21T08%3A31%3A12.000Z"%2C' +
              '"searchStr"%3A"example+search+string"%2C' +
              '"testIDStr"%3A"example+test+id"%2C' +
              '"artifactIDStr"%3A"snippet"%2C' +
              '"isSearchStrRegex"%3Atrue%7D',
          ],
        }}
      >
        <LogSearch project="proj" />
      </FakeContextProvider>,
    );
    const searchStringSelectEle = screen.getByTestId('Search string select');
    expect(searchStringSelectEle).toHaveValue('Regex match:');
    const searchStringInputEle = screen.getByTestId('Search string input');
    expect(searchStringInputEle).toHaveValue('example search string');

    const testIDSelectEle = screen.getByTestId('Test ID select');
    expect(testIDSelectEle).toHaveValue('Exact equal:');
    const testIDInputEle = screen.getByTestId('Test ID input');
    expect(testIDInputEle).toHaveValue('example test id');

    const artifactIDSelectEle = screen.getByTestId('Log file select');
    expect(artifactIDSelectEle).toHaveValue('Exact equal:');
    const artifactIDInputEle = screen.getByTestId('Log file input');
    expect(artifactIDInputEle).toHaveValue('snippet');

    const fromTimeInputEle = screen.getByLabelText('From (UTC)');
    expect(fromTimeInputEle).toHaveValue('07/18/2024 08:31 AM');
    const toTimeInputEle = screen.getByLabelText('To (UTC)');
    expect(toTimeInputEle).toHaveValue('07/21/2024 08:31 AM');
  });

  it('can save form data to search param', () => {
    const urlCallback = jest.fn();
    render(
      <FakeContextProvider mountedPath="/">
        <LogSearch project="proj" />
        <URLObserver callback={urlCallback} />
      </FakeContextProvider>,
    );

    const searchStringSelectEle = screen.getByTestId('Search string select');
    fireEvent.change(searchStringSelectEle, {
      target: { value: 'Regex match:' },
    });
    const searchStringInputEle = screen.getByTestId('Search string input');
    fireEvent.change(searchStringInputEle, {
      target: { value: 'test search string' },
    });

    const testIDSelectEle = screen.getByTestId('Test ID select');
    fireEvent.change(testIDSelectEle, { target: { value: 'Has prefix:' } });
    const testIDInputEle = screen.getByTestId('Test ID input');
    fireEvent.change(testIDInputEle, {
      target: { value: 'test test id prefix' },
    });

    const artifactIDSelectEle = screen.getByTestId('Log file select');
    fireEvent.change(artifactIDSelectEle, {
      target: { value: 'Exact equal:' },
    });
    const artifactIDInputEle = screen.getByTestId('Log file input');
    fireEvent.change(artifactIDInputEle, {
      target: { value: 'test artifact id' },
    });

    const fromTimeInputEle = screen.getByLabelText('From (UTC)');
    fireEvent.change(fromTimeInputEle, {
      target: { value: '06/10/2024 12:00 AM' },
    });
    const toTimeInputEle = screen.getByLabelText('To (UTC)');
    fireEvent.change(toTimeInputEle, {
      target: { value: '06/12/2024 11:00 PM' },
    });
    fireEvent.click(screen.getByText('Search'));

    expect(urlCallback).toHaveBeenLastCalledWith(
      expect.objectContaining({
        search: {
          filter: JSON.stringify({
            testIDStr: 'test test id prefix',
            isTestIDStrPrefix: true,
            artifactIDStr: 'test artifact id',
            isArtifactIDStrPrefix: false,
            searchStr: 'test search string',
            isSearchStrRegex: true,
            startTime: DateTime.fromFormat(
              '06/10/2024 12:00 AM',
              'MM/dd/yyyy t',
            )
              .toUTC()
              .toISO(),
            endTime: DateTime.fromFormat('06/12/2024 11:00 PM', 'MM/dd/yyyy t')
              .toUTC()
              .toISO(),
          }),
        },
      }),
    );
  });
});
