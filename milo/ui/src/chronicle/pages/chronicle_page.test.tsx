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

import { render, screen, waitFor } from '@testing-library/react';
import { useParams } from 'react-router';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChroniclePage } from './chronicle_page';

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: jest.fn(),
}));

const mockUseParams = jest.mocked(useParams);

describe('ChroniclePage login warning when failing to retrieve content', () => {
  beforeEach(() => {
    global.fetch = jest.fn() as unknown as typeof fetch;
    mockUseParams.mockReturnValue({ workplanId: 'wp-access-denied' });
  });

  afterEach(() => {
    jest.resetAllMocks();
  });

  it('displays warning alert encouraging login when user is anonymous and all backends return access denied', async () => {
    const mockFetch = jest.mocked(global.fetch);
    // All environments return 403 (PERMISSION_DENIED -> DetectionErrorType.NoAccess)
    mockFetch.mockResolvedValue({
      ok: false,
      status: 403,
      text: async () => 'Permission Denied',
    } as unknown as Response);

    render(
      <FakeContextProvider>
        <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
          <ChroniclePage />
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Authentication Required')).toBeInTheDocument();
    });

    expect(
      screen.getByText(/You are not logged in\. Please/),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'log in' })).toBeInTheDocument();
    expect(screen.queryByText('Access Denied')).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        /Access was denied to one or more Turbo CI environments when searching for workplan/,
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        /Warning: The following environments could not be checked due to timeouts\/errors/,
      ),
    ).not.toBeInTheDocument();
  });

  it('displays warning encouraging login when user is anonymous and all backends return access denied', async () => {
    const mockFetch = jest.mocked(global.fetch);
    // All backends return 403
    mockFetch.mockImplementation(async () => {
      return {
        ok: false,
        status: 403,
        text: async () => 'Permission Denied',
      } as unknown as Response;
    });

    render(
      <FakeContextProvider>
        <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
          <ChroniclePage />
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Authentication Required')).toBeInTheDocument();
    });

    expect(
      screen.getByText(/You are not logged in\. Please/),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'log in' })).toBeInTheDocument();
    expect(screen.queryByText('Access Denied')).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        /Access was denied to one or more Turbo CI environments when searching for workplan/,
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        /Warning: The following environments could not be checked due to timeouts\/errors/,
      ),
    ).not.toBeInTheDocument();
  });

  it('displays standard error note without logout prompt when user is logged in and backends return general access denied', async () => {
    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockResolvedValue({
      ok: false,
      status: 403,
      text: async () => 'Permission Denied',
    } as unknown as Response);

    render(
      <FakeContextProvider>
        <FakeAuthStateProvider
          value={{
            identity: 'user:loggedInUser@example.com',
            email: 'loggedInUser@example.com',
          }}
        >
          <ChroniclePage />
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Access Denied')).toBeInTheDocument();
    });

    expect(
      screen.getByText(
        /Access was denied to one or more Turbo CI environments when searching for workplan wp-access-denied\./,
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('Authentication Required'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        /This may be because your authentication scopes are invalid or outdated\./,
      ),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('link', { name: 'log out' }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(
        /Warning: The following environments could not be checked due to timeouts\/errors/,
      ),
    ).toBeInTheDocument();
    expect(screen.getAllByText(/no access/).length).toBeGreaterThan(0);
  });

  it('displays "You Must Re-Login" alert encouraging logout when user is logged in and backends return scope error', async () => {
    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockResolvedValue({
      ok: false,
      status: 403,
      text: async () => 'Request had insufficient authentication scopes.',
    } as unknown as Response);

    render(
      <FakeContextProvider>
        <FakeAuthStateProvider
          value={{
            identity: 'user:loggedInUser@example.com',
            email: 'loggedInUser@example.com',
          }}
        >
          <ChroniclePage />
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('You Must Re-Login')).toBeInTheDocument();
    });

    expect(
      screen.getByText(/Your authentication scopes are invalid or outdated\./),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'log out' })).toBeInTheDocument();
    expect(
      screen.queryByText('Authentication Required'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        /Warning: The following environments could not be checked due to timeouts\/errors/,
      ),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/invalid scopes/)).not.toBeInTheDocument();
  });

  it('displays Workplan Not Found when all backends return 404 (not found)', async () => {
    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockResolvedValue({
      ok: false,
      status: 404,
      text: async () => 'Not Found',
    } as unknown as Response);

    render(
      <FakeContextProvider>
        <FakeAuthStateProvider
          value={{
            identity: 'user:loggedInUser@example.com',
            email: 'loggedInUser@example.com',
          }}
        >
          <ChroniclePage />
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Workplan Not Found')).toBeInTheDocument();
    });

    expect(
      screen.getByText(
        'Workplan wp-access-denied could not be found in any of the Turbo CI environments.',
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('Authentication Required'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText(
        /Warning: The following environments could not be checked due to timeouts\/errors/,
      ),
    ).not.toBeInTheDocument();
  });

  it('displays File a bug/FR link', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'demo' });

    render(
      <FakeContextProvider>
        <FakeAuthStateProvider
          value={{
            identity: 'user:loggedInUser@example.com',
            email: 'loggedInUser@example.com',
          }}
        >
          <ChroniclePage />
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      const link = screen.getByRole('link', { name: 'File a bug/FR' });
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute('href', 'http://go/turbo-ci-bug');
    });
  });
});
