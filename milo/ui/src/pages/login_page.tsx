// Copyright 2020 The LUCI Authors.
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

import { Link } from '@mui/material';
import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { getLoginUrl } from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

/**
 * Prompts the user to login.
 * Once logged in, redirects to
 *   - URL specified in 'redirect' search param, or
 *   - root.
 * in that order.
 */
export function LoginPage() {
  const navigate = useNavigate();
  const authState = useAuthState();
  const [searchParam] = useSyncedSearchParams();
  const redirectTo = searchParam.get('redirect') || '/';

  const isLoggedIn = authState.identity !== ANONYMOUS_IDENTITY;

  // Perform redirection directly if the user is already logged in. This might
  // happen if the user logged in via another browser tab and the auth state is
  // refreshed.
  useEffect(() => {
    if (!isLoggedIn) {
      return;
    }
    navigate(redirectTo);
  }, [isLoggedIn, navigate, redirectTo]);

  return (
    <div css={{ margin: '8px 16px' }}>
      You must{' '}
      <Link
        href={getLoginUrl(redirectTo)}
        css={{ textDecoration: 'underline', cursor: 'pointer' }}
      >
        login
      </Link>{' '}
      to see anything useful.
    </div>
  );
}
