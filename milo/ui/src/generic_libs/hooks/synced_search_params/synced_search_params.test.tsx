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

import { fireEvent, render, screen } from '@testing-library/react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useSyncedSearchParams } from './hooks';

function TestComponent() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  return (
    <>
      <div data-testid="search-params-display">{searchParams.toString()}</div>
      <button
        data-testid="trigger-update"
        onClick={() => {
          setSearchParams((prev) => {
            const next = new URLSearchParams(prev);
            next.set('key1', 'val1');
            return next;
          });

          setSearchParams((prev) => {
            const next = new URLSearchParams(prev);
            next.set('key2', 'val2');
            return next;
          });

          setSearchParams((prev) => {
            const next = new URLSearchParams(prev);
            next.set('key3', 'val3');
            return next;
          });
        }}
      ></button>
    </>
  );
}

describe('useSyncedSearchParams', () => {
  it('should not discard results from intermediate updaters', () => {
    render(
      <FakeContextProvider>
        <TestComponent />
      </FakeContextProvider>,
    );

    const updaterButton = screen.getByTestId('trigger-update');
    const display = screen.getByTestId('search-params-display');

    expect(display).toHaveTextContent('');

    fireEvent.click(updaterButton);
    expect(display).toHaveTextContent('key1=val1&key2=val2&key3=val3');
  });
});
