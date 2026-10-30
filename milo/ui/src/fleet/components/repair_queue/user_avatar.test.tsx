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

import { act, render, screen } from '@testing-library/react';

import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import {
  AVATAR_COLORS,
  getAvatarColor,
  getInitial,
  UserAvatar,
} from './user_avatar';

describe('user_avatar helpers & component', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('getInitial', () => {
    it('extracts uppercase initial from name', () => {
      expect(getInitial('Andrew Miller', 'andrew@google.com')).toBe('A');
    });

    it('falls back to email when name is empty or whitespace', () => {
      expect(getInitial('', 'beatrice@google.com')).toBe('B');
      expect(getInitial('   ', 'charlie@google.com')).toBe('C');
    });

    it('strips user: prefix when computing initial', () => {
      expect(getInitial('user:david@google.com')).toBe('D');
      expect(getInitial(undefined, 'user:ellen@google.com')).toBe('E');
    });

    it('returns ? when no valid text is provided', () => {
      expect(getInitial()).toBe('?');
      expect(getInitial('', '')).toBe('?');
      expect(getInitial('user:', '')).toBe('?');
    });
  });

  describe('getAvatarColor', () => {
    it('returns a color from AVATAR_COLORS consistently', () => {
      const color = getAvatarColor('andrew@google.com');
      expect(AVATAR_COLORS).toContain(color);
      expect(getAvatarColor('andrew@google.com')).toBe(color);
    });

    it('produces the same color regardless of user: prefix or outer whitespace', () => {
      expect(getAvatarColor('user:tech1@google.com')).toBe(
        getAvatarColor('tech1@google.com'),
      );
      expect(getAvatarColor('  tech1@google.com  ')).toBe(
        getAvatarColor('tech1@google.com'),
      );
    });

    it('produces distinct colors for polarczyk and pietroscutta', () => {
      expect(getAvatarColor('polarczyk@google.com')).not.toBe(
        getAvatarColor('pietroscutta@google.com'),
      );
    });
  });

  describe('<UserAvatar />', () => {
    it('renders initial derived from name/email', () => {
      render(<UserAvatar name="Alice Smith" email="alice@google.com" />);
      expect(screen.getByText('A')).toBeInTheDocument();
    });

    it('renders fallback initial from id if name and email are missing', () => {
      render(<UserAvatar id="bob123" />);
      expect(screen.getByText('B')).toBeInTheDocument();
    });

    it('allows custom children override', () => {
      render(<UserAvatar name="Alice Smith">AS</UserAvatar>);
      expect(screen.getByText('AS')).toBeInTheDocument();
    });

    it('renders photo with no-referrer policy and falls back to monogram on load error', () => {
      let createdImage: HTMLImageElement | undefined;
      const NativeImage = window.Image;
      jest.spyOn(window, 'Image').mockImplementation(() => {
        createdImage = new NativeImage();
        return createdImage;
      });

      render(
        <FakeAuthStateProvider
          value={{
            identity: 'user:viewer@google.com',
            email: 'viewer@google.com',
          }}
        >
          <UserAvatar name="Alice Smith" email="user:alice@google.com" />
        </FakeAuthStateProvider>,
      );

      const img = screen.getByRole('img', { name: 'Alice Smith' });
      expect(img).toHaveAttribute(
        'src',
        'https://moma-teams-photos.corp.google.com/photos/alice%40google.com?sz=96&type=preferred&no_default_image=true&force_no_redirect=true',
      );
      expect(img).toHaveAttribute('referrerpolicy', 'no-referrer');

      act(() => {
        createdImage?.onerror?.(new Event('error'));
      });
      expect(screen.getByText('A')).toBeInTheDocument();
    });

    it('respects an explicit src prop override', () => {
      render(
        <UserAvatar
          name="Alice Smith"
          email="alice@google.com"
          src="https://example.com/custom.png"
        />,
      );

      expect(screen.getByRole('img', { name: 'Alice Smith' })).toHaveAttribute(
        'src',
        'https://example.com/custom.png',
      );
    });
  });
});
