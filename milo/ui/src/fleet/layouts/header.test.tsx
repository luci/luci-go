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

import { render, screen } from '@testing-library/react';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { Header } from './header';

jest.mock('@/common/components/auth_state_provider');

describe('Header', () => {
  it('When not logged in should display login button', async () => {
    (useAuthState as jest.Mock).mockReturnValue({
      identity: '',
    });
    const { rerender } = render(
      <FakeContextProvider>
        <Header sidebarOpen={false} setSidebarOpen={() => {}} />,
      </FakeContextProvider>,
    );

    expect(screen.getByText('Login')).toBeInTheDocument();

    (useAuthState as jest.Mock).mockReturnValue({
      identity: ANONYMOUS_IDENTITY,
    });
    rerender(
      <FakeContextProvider>
        <Header sidebarOpen={false} setSidebarOpen={() => {}} />,
      </FakeContextProvider>,
    );
    expect(screen.getByText('Login')).toBeInTheDocument();
  });

  it('When logged in should display the avatar', async () => {
    (useAuthState as jest.Mock).mockReturnValue({
      email: 'googler@google.com',
      identity: 'id:123456',
      picture: 'avatar_url',
    });
    render(
      <FakeContextProvider>
        <Header sidebarOpen={false} setSidebarOpen={() => {}} />,
      </FakeContextProvider>,
    );
    expect(screen.getByLabelText('avatar')).toBeInTheDocument();
  });
});
