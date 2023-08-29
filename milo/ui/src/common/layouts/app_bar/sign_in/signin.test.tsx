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

import { render, screen } from '@testing-library/react';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SignIn } from './signin';

describe('SignIn', () => {
  it('given no identity or anonymouse identity, should display login button', async () => {
    const { rerender } = render(
      <FakeContextProvider>
        <SignIn />
      </FakeContextProvider>
    );
    expect(screen.getByText('Login')).toBeInTheDocument();

    rerender(
      <FakeContextProvider>
        <SignIn identity={ANONYMOUS_IDENTITY} />
      </FakeContextProvider>
    );
    expect(screen.getByText('Login')).toBeInTheDocument();
  });

  it('given an identity, should display email and logout button', async () => {
    render(
      <FakeContextProvider>
        <SignIn
          email="googler@google.com"
          identity="id:123456"
          picture="avatar_url"
        />
      </FakeContextProvider>
    );
    expect(screen.getByText('googler@google.com')).toBeInTheDocument();
    expect(screen.getByLabelText('avatar')).toBeInTheDocument();
    expect(screen.getByLabelText('Logout button')).toBeInTheDocument();
  });
});
