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

import { render, screen } from '@testing-library/react';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { LoggedInBoundary } from './logged_in_boundary';

describe('<LoggedInBoundary />', () => {
  it('renders the sign-in required page when the user is anonymous', () => {
    render(
      <FakeContextProvider
        mountedPath="/ui/fleet/p/chromeos/devices"
        routerOptions={{
          initialEntries: ['/ui/fleet/p/chromeos/devices?filters=dut_state'],
        }}
      >
        <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
          <LoggedInBoundary>
            <div data-testid="protected-content">Protected Fleet Content</div>
          </LoggedInBoundary>
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('fleet-not-signed-in-page')).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: 'Sign in to Fleet Console' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText('/ui/fleet/p/chromeos/devices?filters=dut_state'),
    ).toBeInTheDocument();

    const signInLink = screen.getByRole('link', { name: /sign in/i });
    expect(signInLink).toHaveAttribute(
      'href',
      expect.stringContaining(
        encodeURIComponent('/ui/fleet/p/chromeos/devices?filters=dut_state'),
      ),
    );
    expect(screen.queryByTestId('protected-content')).not.toBeInTheDocument();
  });

  it('falls back to /ui/fleet redirect target when mounted at root path', () => {
    render(
      <FakeContextProvider
        mountedPath="/"
        routerOptions={{
          initialEntries: ['/'],
        }}
      >
        <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
          <LoggedInBoundary>
            <div data-testid="protected-content">Protected Fleet Content</div>
          </LoggedInBoundary>
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('/ui/fleet')).toBeInTheDocument();
    const signInLink = screen.getByRole('link', { name: /sign in/i });
    expect(signInLink).toHaveAttribute(
      'href',
      expect.stringContaining(encodeURIComponent('/ui/fleet')),
    );
  });

  it('renders children when the user is signed in', () => {
    render(
      <FakeContextProvider>
        <FakeAuthStateProvider
          value={{
            identity: 'user:tester@google.com',
            email: 'tester@google.com',
          }}
        >
          <LoggedInBoundary>
            <div data-testid="protected-content">Protected Fleet Content</div>
          </LoggedInBoundary>
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('protected-content')).toBeInTheDocument();
    expect(
      screen.queryByTestId('fleet-not-signed-in-page'),
    ).not.toBeInTheDocument();
  });
});
