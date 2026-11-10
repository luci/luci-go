// Copyright 2025 The LUCI Authors.
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

import { PageNotFoundPage } from './not_found_page';

describe('<PageNotFoundPage />', () => {
  it('shows the sign-in notice when the user is not signed in', async () => {
    render(
      <FakeAuthStateProvider value={{ identity: ANONYMOUS_IDENTITY }}>
        <PageNotFoundPage />
      </FakeAuthStateProvider>,
    );

    expect(screen.getByText(/Page not found/)).toBeVisible();
    expect(
      screen.getByText(
        'Some pages require sign-in or access permissions to view. Sign in to continue.',
      ),
    ).toBeVisible();

    const signInButton = screen.getByRole('link', { name: /Sign in/i });
    expect(signInButton).toBeVisible();
    expect(signInButton).toHaveAttribute(
      'href',
      expect.stringContaining('/auth/openid/login'),
    );
  });

  it('hides the sign-in notice when the user is already signed in', async () => {
    render(
      <FakeAuthStateProvider
        value={{
          identity: 'user:tester@google.com',
          email: 'tester@google.com',
        }}
      >
        <PageNotFoundPage />
      </FakeAuthStateProvider>,
    );

    expect(screen.getByText(/Page not found/)).toBeVisible();
    expect(
      screen.queryByText(
        /Some pages require sign-in or access permissions to view/i,
      ),
    ).not.toBeInTheDocument();
  });
});
