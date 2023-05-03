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
import { useLocation, useNavigate } from 'react-router-dom';

import { useAuthState } from '../components/auth_state_provider';
import { ANONYMOUS_IDENTITY } from '../libs/auth_state';
import { getLoginUrl } from '../libs/url_utils';

/**
 * Prompts the user to login.
 * Once logged in, redirects to
 *   - URL specified in 'redirect' search param, or
 *   - root.
 * in that order.
 */
export function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const authState = useAuthState();

  const isLoggedIn = ![undefined, ANONYMOUS_IDENTITY].includes(authState.identity);

  useEffect(() => {
    if (!isLoggedIn) {
      return;
    }
    const redirect = new URLSearchParams(window.location.search).get('redirect');
    navigate(redirect || '/');
  }, [isLoggedIn]);

  return (
    <div css={{ margin: '8px 16px' }}>
      You must{' '}
      <Link
        href={getLoginUrl(location.pathname + location.search + location.hash)}
        css={{ textDecoration: 'underline', cursor: 'pointer' }}
      >
        login
      </Link>{' '}
      to see anything useful.
    </div>
  );
}
