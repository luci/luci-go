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

import { cleanup, render, screen } from '@testing-library/react';
import { ReactNode } from 'react';

import { resetSilence, silence } from '@/testing_tools/console_filter';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestAuthStateContext } from '../auth_state_provider';

import { RouteErrorDisplay } from './route_error_display';

const SILENCED_ERROR_MAGIC_STRING = ' <602fd9c>';

function TestFailureComponent(): ReactNode {
  throw new Error(
    'error from test failure component' + SILENCED_ERROR_MAGIC_STRING,
  );
}

describe('<RouteErrorDisplay />', () => {
  beforeEach(() => {
    // Silence error related to the error we thrown. Note that the following
    // method isn't able to silence the error from React for some reason.
    // ```
    // jest.spyOn(console, 'error').mockImplementation(() => {});
    // ```
    silence('error', (...params) =>
      `${params}`.includes(SILENCED_ERROR_MAGIC_STRING),
    );
    silence('error', (err) =>
      `${err}`.includes(
        'React will try to recreate this component tree from scratch using the error boundary you provided',
      ),
    );
  });

  afterEach(() => {
    resetSilence();
    cleanup();
  });

  it('does not require any excessive contexts', () => {
    render(
      <FakeContextProvider
        errorElement={
          <TestAuthStateContext.Provider value={undefined}>
            <RouteErrorDisplay />
          </TestAuthStateContext.Provider>
        }
      >
        <TestFailureComponent />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('error from test failure component', { exact: false }),
    ).toBeInTheDocument();
  });
});
