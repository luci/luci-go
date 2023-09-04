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

import { useEffect, useRef } from 'react';
import { FallbackProps } from 'react-error-boundary';
import { useLatest } from 'react-use';

import { useAuthState } from '@/common/components/auth_state_provider';

import { ErrorDisplay } from './error_display';

/**
 * An error fallback that renders the error with a "retry" button.
 * It also retries rendering automatically when appropriate.
 */
export function RecoverableErrorFallback({
  error,
  resetErrorBoundary,
}: FallbackProps) {
  const err = error instanceof Error ? error : new Error(`${error}`);

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

  return <ErrorDisplay error={err} onTryAgain={() => resetErrorBoundary()} />;
}
