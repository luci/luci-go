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
            '/?filter=%7B"searchStr"%3A"example+search+string"%2C' +
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
          }),
        },
      }),
    );
  });
});
