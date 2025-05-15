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
import { useNavigate } from 'react-router';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { getLoginUrl } from '@/common/tools/url_utils';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
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

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="login">
      <RecoverableErrorBoundary
        // We cannot use `<RecoverableErrorBoundary />` in `errorElement`
        // because it (react-router) doesn't support error recovery.
        //
        // We handle the error at child level rather than at the parent level
        // because we want the error state to be reset when the user navigates
        // to a sibling view, which does not happen if the error is handled by
        // the parent (without additional logic).
        // The downside of this model is that we do not have a central place for
        // error handling, which is somewhat mitigated by applying the same
        // error boundary on all child routes.
        // The upside is that the error is naturally reset on route changes.
        //
        // A unique `key` is needed to ensure the boundary is not reused when
        // the user navigates to a sibling view. The error will be naturally
        // discarded as the route is unmounted.
        key="login"
      >
        <LoginPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
