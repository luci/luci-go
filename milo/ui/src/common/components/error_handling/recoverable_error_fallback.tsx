// Copyright 2023 The LUCI Authors.
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

import { GrpcError } from '@chopsui/prpc-client';
import { Link } from '@mui/material';
import { useEffect, useRef } from 'react';
import { FallbackProps } from 'react-error-boundary';
import { useLocation } from 'react-router';
import { useLatest } from 'react-use';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { POTENTIAL_PERM_ERROR_CODES } from '@/common/constants/rpc';
import { getLoginUrl } from '@/common/tools/url_utils';

import { ErrorDisplay } from './error_display';

/**
 * An error fallback that renders the error with a "retry" button.
 * It also retries rendering automatically when appropriate.
 */
export function RecoverableErrorFallback({
  error,
  resetErrorBoundary,
}: FallbackProps) {
  const location = useLocation();
  const resetRef = useLatest(resetErrorBoundary);

  // A lot of the errors are caused by users lacking permissions. Reset the
  // error when the user identity changes.
  // Note that we intentionally do not check whether the error is a query/
  // permission error. This is because the error could surface in other forms
  // (e.g. missing data).
  const authState = useAuthState();
  const initialIdentity = useRef(authState.identity);
  useEffect(() => {
    if (initialIdentity.current === authState.identity) {
      return;
    }
    resetRef.current();
  }, [authState.identity, resetRef]);

  const err = error instanceof Error ? error : new Error(`${error}`);
  const shouldAskToLogin =
    initialIdentity.current === ANONYMOUS_IDENTITY &&
    err instanceof GrpcError &&
    POTENTIAL_PERM_ERROR_CODES.includes(err.code);

  return (
    <ErrorDisplay
      error={err}
      instruction={
        shouldAskToLogin && (
          <b>
            You may need to{' '}
            <Link
              href={getLoginUrl(
                location.pathname + location.search + location.hash,
              )}
            >
              login
            </Link>{' '}
            to view the page.
          </b>
        )
      }
      showFileBugButton={initialIdentity.current !== ANONYMOUS_IDENTITY}
      onTryAgain={() => resetErrorBoundary()}
    />
  );
}
