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
      expect(screen.getByText('Workplan Not Found')).toBeInTheDocument();
    });

    expect(screen.getByText('Authentication Required')).toBeInTheDocument();
    expect(
      screen.getByText(
        /Access was denied for this workplan, and you are not currently logged in/,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'logging in' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Note: The following environments could not be checked/),
    ).toBeInTheDocument();
  });

  it('displays warning encouraging login when user is anonymous and backends return a mix of access denied and not found', async () => {
    const mockFetch = jest.mocked(global.fetch);
    // Prod returns 403, all other backends return 404
    mockFetch.mockImplementation(async (url: RequestInfo | URL) => {
      if (url.toString().includes('turboci.pa.googleapis.com')) {
        return {
          ok: false,
          status: 403,
          text: async () => 'Permission Denied',
        } as unknown as Response;
      }
      return {
        ok: false,
        status: 404,
        text: async () => 'Not Found',
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
      expect(screen.getByText('Workplan Not Found')).toBeInTheDocument();
    });

    expect(screen.getByText('Authentication Required')).toBeInTheDocument();
    expect(
      screen.getByText(
        /Access was denied for this workplan, and you are not currently logged in/,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'logging in' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Note: The following environments could not be checked/),
    ).toBeInTheDocument();
  });

  it('displays standard error note without login warning alert when user is logged in and all backends return access denied', async () => {
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
      expect(screen.getByText('Workplan Not Found')).toBeInTheDocument();
    });

    expect(
      screen.queryByText('Authentication Required'),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(/Note: The following environments could not be checked/),
    ).toBeInTheDocument();
    expect(screen.getByText(/no access/)).toBeInTheDocument();
  });
});
