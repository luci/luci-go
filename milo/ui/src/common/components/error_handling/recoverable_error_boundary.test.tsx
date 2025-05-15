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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { useQuery } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { userEvent } from '@testing-library/user-event';
import { act } from 'react';
import { ReactNode } from 'react';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { resetSilence, silence } from '@/testing_tools/console_filter';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { RecoverableErrorBoundary } from './recoverable_error_boundary';

const SILENCED_ERROR_MAGIC_STRING = ' <cdaac21>';

function IdentityTestComponent() {
  const { identity } = useAuthState();

  const { data, error } = useQuery({
    queryKey: ['test-key', identity],
    queryFn: () => {
      if (identity === ANONYMOUS_IDENTITY) {
        throw new Error('cannot be anonymous' + SILENCED_ERROR_MAGIC_STRING);
      }
      return `Hello ${identity}`;
    },
  });

  if (error) {
    throw error;
  }

  return <>{data}</>;
}

function PermissionErrorComponent(): ReactNode {
  throw new GrpcError(
    RpcCode.PERMISSION_DENIED,
    'error display' + SILENCED_ERROR_MAGIC_STRING,
  );
}

function InternalErrorComponent(): ReactNode {
  throw new GrpcError(
    RpcCode.INTERNAL,
    'error display' + SILENCED_ERROR_MAGIC_STRING,
  );
}

describe('<RecoverableErrorBoundary />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    // Silence error related to the error we thrown. Note that the following
    // method isn't able to silence the error from React for some reason.
    // ```
    // jest.spyOn(console, 'error').mockImplementation(() => {});
    // ```
    // eslint-disable-next-line no-console
    silence('error', (err) => `${err}`.includes(SILENCED_ERROR_MAGIC_STRING));
    silence('error', (err) =>
      `${err}`.includes(
        'React will try to recreate this component tree from scratch using the error boundary you provided',
      ),
    );
  });

  afterEach(() => {
    jest.useRealTimers();
    resetSilence();
    cleanup();
  });

  // TODO(b/416138280): fix test.
  // eslint-disable-next-line jest/no-disabled-tests
  it.skip('can recover from error when user identity changes', async () => {
    const { rerender } = render(
      <FakeContextProvider>
        <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
          <RecoverableErrorBoundary>
            <IdentityTestComponent />
          </RecoverableErrorBoundary>
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('cannot be anonymous');

    rerender(
      <FakeContextProvider>
        <FakeAuthStateProvider value={{ identity: 'user:user@google.com' }}>
          <RecoverableErrorBoundary>
            <IdentityTestComponent />
          </RecoverableErrorBoundary>
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());

    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByText('Hello user:user@google.com')).toBeInTheDocument();
  });

  it('can recover from error when retry button is clicked', async () => {
    let shouldThrowError = true;

    function RetryTestComponent() {
      const { data, error } = useQuery({
        queryKey: ['test-key'],
        queryFn: () => {
          if (shouldThrowError) {
            throw new Error(
              'encountered an error' + SILENCED_ERROR_MAGIC_STRING,
            );
          }
          return `No error`;
        },
      });

      if (error) {
        throw error;
      }

      return <>{data}</>;
    }

    const { rerender } = render(
      <FakeContextProvider>
        <RecoverableErrorBoundary>
          <RetryTestComponent />
        </RecoverableErrorBoundary>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('encountered an error');

    shouldThrowError = false;

    rerender(
      <FakeContextProvider>
        <RecoverableErrorBoundary>
          <RetryTestComponent />
        </RecoverableErrorBoundary>
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('encountered an error');

    userEvent.click(screen.getByText('Try Again'));
    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());

    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByText('No error')).toBeInTheDocument();
  });

  it('displays login instruction when needed', async () => {
    render(
      <FakeContextProvider
        mountedPath="/ui/*"
        routerOptions={{ initialEntries: ['/ui/link/to/current/page'] }}
      >
        <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
          <RecoverableErrorBoundary>
            <PermissionErrorComponent />
          </RecoverableErrorBoundary>
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('error display');
    expect(screen.getByText('login')).toHaveAttribute(
      'href',
      '/auth/openid/login?r=%2Fui%2Flink%2Fto%2Fcurrent%2Fpage',
    );
  });

  it('does not displays login instruction when user has signed in', async () => {
    render(
      <FakeContextProvider
        mountedPath="/ui/*"
        routerOptions={{ initialEntries: ['/ui/link/to/current/page'] }}
      >
        <FakeAuthStateProvider value={{ identity: 'user:user@google.com' }}>
          <RecoverableErrorBoundary>
            <PermissionErrorComponent />
          </RecoverableErrorBoundary>
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('error display');
    expect(screen.queryByText('login')).not.toBeInTheDocument();
  });

  it('does not displays login instruction when its not permission error', async () => {
    render(
      <FakeContextProvider
        mountedPath="/ui/*"
        routerOptions={{ initialEntries: ['/ui/link/to/current/page'] }}
      >
        <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
          <RecoverableErrorBoundary>
            <InternalErrorComponent />
          </RecoverableErrorBoundary>
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('error display');
    expect(screen.queryByText('login')).not.toBeInTheDocument();
  });
});
