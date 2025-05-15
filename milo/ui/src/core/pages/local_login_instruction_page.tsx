// Copyright 2024 The LUCI Authors.
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

import { Alert, Box, Link } from '@mui/material';
import { useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { Link as RouterLink, useNavigate } from 'react-router';
import { useInterval } from 'react-use';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import {
  AUTH_STATE_QUERY_KEY,
  useAuthState,
} from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

/**
 * Give the developers instruction to login during local development.
 * Once logged in, redirects to
 *   - URL specified in 'redirect' search param, or
 *   - root.
 * in that order.
 */
export function LocalLoginInstructionPage() {
  const navigate = useNavigate();
  const authState = useAuthState();
  const [searchParam] = useSyncedSearchParams();
  const redirectTo = searchParam.get('redirect') || '/';
  const scopes = searchParam.get('scopes');
  if (!scopes) {
    throw new Error('scopes must be defined');
  }

  const isLoggedIn = authState.identity !== ANONYMOUS_IDENTITY;

  // Refresh the auth-state query periodically so we can redirect users to
  // once the login flow is complete.
  const client = useQueryClient();
  useInterval(
    () => client.invalidateQueries({ queryKey: AUTH_STATE_QUERY_KEY }),
    isLoggedIn ? null : 10000,
  );

  // Perform redirection directly if the user is already logged in.
  useEffect(() => {
    if (!isLoggedIn) {
      return;
    }
    navigate(redirectTo);
  }, [isLoggedIn, navigate, redirectTo]);

  const command = `luci-auth login -scopes "${scopes}"`;

  return (
    <div css={{ margin: '8px 16px', fontSize: '16px' }}>
      <p>
        When developing locally, please run the following command in your
        terminal to obtain a session.
      </p>
      <Alert severity="info">
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: '1fr auto',
          }}
        >
          <Box>{command}</Box>
          <CopyToClipboard
            textToCopy={command}
            title="Copy the command."
            sx={{ margin: '5px' }}
          />
        </Box>
      </Alert>
      <p>The page should redirect shortly after you complete the login flow.</p>
      <p>
        If not, click{' '}
        <Link component={RouterLink} to={redirectTo}>
          here
        </Link>{' '}
        to go back to the previous page.
      </p>
      <Alert severity="warning">
        This login flow does not create a http cookie based session. Endpoints
        that requires cookie based authentication (e.g. raw artifact URLs) will
        not work.
      </Alert>
    </div>
  );
}

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="local-login-instruction">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="local-login-instruction"
      >
        <LocalLoginInstructionPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
