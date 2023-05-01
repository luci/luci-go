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
import { observer } from 'mobx-react-lite';
import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import { ANONYMOUS_IDENTITY } from '../libs/auth_state';
import { getLoginUrl } from '../libs/url_utils';
import { useStore } from '../store';

/**
 * Prompts the user to login.
 * Once logged in, redirects to
 *   - URL specified in 'redirect' search param, or
 *   - root.
 * in that order.
 */
export const LoginPage = observer(() => {
  const store = useStore();
  const navigate = useNavigate();
  const location = useLocation();

  const isLoggedIn = ![undefined, ANONYMOUS_IDENTITY].includes(store.authState.userIdentity);

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
});
