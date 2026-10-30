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

import { useUserPhoto, UseUserPhotoOptions } from './use_user_photo';

const renderWithAuthState = (
  options: UseUserPhotoOptions | undefined,
  authState?: AuthState,
) =>
  renderHook(() => useUserPhoto(options), {
    wrapper: authState
      ? ({ children }: { children: React.ReactNode }) => (
          <FakeAuthStateProvider value={authState}>
            <>{children}</>
          </FakeAuthStateProvider>
        )
      : undefined,
  });

describe('useUserPhoto', () => {
  it('returns Moma photo URL, alt label, and no-referrer slotProps for @google.com users', () => {
    const { result } = renderWithAuthState(
      { name: 'Alice Smith', email: '  USER:Alice@Google.com ' },
      {
        identity: 'user:viewer@google.com',
        email: 'viewer@google.com',
      },
    );

    expect(result.current.src).toBe(
      'https://moma-teams-photos.corp.google.com/photos/alice%40google.com?sz=96&type=preferred&no_default_image=true&force_no_redirect=true',
    );
    expect(result.current.alt).toBe('Alice Smith');
    expect(result.current.slotProps?.img).toEqual({
      referrerPolicy: 'no-referrer',
    });
  });

  it('prefers https viewer picture from auth state for the current user', () => {
    const { result } = renderWithAuthState(
      { email: 'user:viewer@google.com' },
      {
        identity: 'user:viewer@google.com',
        email: 'viewer@google.com',
        picture: 'https://lh3.googleusercontent.com/a/viewer-photo',
      },
    );

    expect(result.current.src).toBe(
      'https://lh3.googleusercontent.com/a/viewer-photo',
    );
    expect(result.current.alt).toBe('viewer@google.com');
  });

  it('ignores non-https viewer picture URLs and falls back to Moma URL', () => {
    const { result } = renderWithAuthState(
      { email: 'viewer@google.com' },
      {
        identity: 'user:viewer@google.com',
        email: 'viewer@google.com',
        picture: 'http://insecure.example.com/photo.png',
      },
    );

    expect(result.current.src).toBe(
      'https://moma-teams-photos.corp.google.com/photos/viewer%40google.com?sz=96&type=preferred&no_default_image=true&force_no_redirect=true',
    );
  });

  it('returns undefined src when auth provider is absent or viewer is not @google.com', () => {
    expect(
      renderWithAuthState({ email: 'alice@google.com' }).result.current.src,
    ).toBeUndefined();
    expect(
      renderWithAuthState(
        { email: 'alice@google.com' },
        { identity: 'anonymous:anonymous' },
      ).result.current.src,
    ).toBeUndefined();
    expect(
      renderWithAuthState(
        { email: 'alice@google.com' },
        {
          identity: 'user:external@chromium.org',
          email: 'external@chromium.org',
        },
      ).result.current.src,
    ).toBeUndefined();
  });

  it('returns undefined src for empty, external, or non-user target identities', () => {
    const corpViewer: AuthState = {
      identity: 'user:viewer@google.com',
      email: 'viewer@google.com',
    };

    expect(
      renderWithAuthState(undefined, corpViewer).result.current,
    ).toMatchObject({
      src: undefined,
      alt: undefined,
    });
    expect(
      renderWithAuthState({ email: '' }, corpViewer).result.current,
    ).toMatchObject({
      src: undefined,
      alt: undefined,
    });
    expect(
      renderWithAuthState({ email: 'partner@external.org' }, corpViewer).result
        .current.src,
    ).toBeUndefined();
    expect(
      renderWithAuthState({ email: 'bot:ci@google.com' }, corpViewer).result
        .current.src,
    ).toBeUndefined();
  });
});
