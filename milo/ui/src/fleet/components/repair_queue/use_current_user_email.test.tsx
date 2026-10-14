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

import { renderHook } from '@testing-library/react';

import { AuthState } from '@/common/api/auth_state';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import {
  UNKNOWN_USER_EMAIL,
  useCurrentUserEmail,
} from './use_current_user_email';

const renderWithAuthState = (authState: AuthState) =>
  renderHook(() => useCurrentUserEmail(), {
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <FakeAuthStateProvider value={authState}>
        <>{children}</>
      </FakeAuthStateProvider>
    ),
  });

describe('useCurrentUserEmail', () => {
  it('prefers the auth state email', () => {
    const { result } = renderWithAuthState({
      identity: 'user:other@example.com',
      email: '  tech@example.com ',
    });

    expect(result.current).toBe('tech@example.com');
  });

  it('falls back to the identity with the user: prefix stripped', () => {
    const { result } = renderWithAuthState({
      identity: 'user:tech@example.com',
      email: '',
    });

    expect(result.current).toBe('tech@example.com');
  });

  it('keeps non-user identities verbatim', () => {
    const { result } = renderWithAuthState({
      identity: 'anonymous:anonymous',
    });

    expect(result.current).toBe('anonymous:anonymous');
  });

  it('falls back to a placeholder when nothing is known', () => {
    const { result } = renderWithAuthState({ identity: ' ' });

    expect(result.current).toBe(UNKNOWN_USER_EMAIL);
  });
});
