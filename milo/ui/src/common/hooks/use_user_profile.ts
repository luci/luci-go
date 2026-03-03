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

import { useQuery } from '@tanstack/react-query';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import {
  TokenType,
  useAuthState,
  useGetAuthToken,
} from '@/common/components/auth_state_provider';

/**
 * Hook to fetch and return the user's profile information from Google's UserInfo API.
 *
 * `useAuthState` provides basic authentication state from luci-auth, including an email
 * and an identity string, but it does not always contain the user's actual name or
 * full Google profile details. This hook fetches the full profile to get the given_name
 * for personalized greetings.
 */
export function useUserProfile() {
  const authState = useAuthState();
  const getAccessToken = useGetAuthToken(TokenType.Access);
  const isAnonymous = authState.identity === ANONYMOUS_IDENTITY;

  const query = useQuery({
    queryKey: ['userProfile', authState.identity],
    queryFn: async () => {
      if (isAnonymous) {
        return null;
      }
      const token = await getAccessToken();
      const res = await fetch('https://www.googleapis.com/oauth2/v3/userinfo', {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!res.ok) {
        throw new Error('Failed to fetch user profile');
      }
      return res.json();
    },
    enabled: !isAnonymous,
    staleTime: Infinity,
  });

  let displayFirstName = '';
  if (query.data?.given_name) {
    displayFirstName = query.data.given_name;
  } else if (authState.email) {
    const firstName = authState.email.split('@')[0];
    displayFirstName = firstName.charAt(0).toUpperCase() + firstName.slice(1);
  }

  return {
    query,
    displayFirstName,
    email: authState.email,
    picture: query.data?.picture || authState.picture,
    isAnonymous,
  };
}
