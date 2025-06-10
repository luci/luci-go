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

import { QueryErrorResetBoundary } from '@tanstack/react-query';
import { ReactNode } from 'react';
import { ErrorBoundary } from 'react-error-boundary';

import { RecoverableErrorFallback } from './recoverable_error_fallback';

export interface RecoverableErrorBoundaryProps {
  readonly children: ReactNode;
  readonly onError?: (error: Error, info: React.ErrorInfo) => void;
}

/**
 * An error boundary that supports various error recovering strategies.
 */
// See https://tanstack.com/query/latest/docs/react/reference/QueryErrorResetBoundary
export function RecoverableErrorBoundary({
  children,
  onError,
}: RecoverableErrorBoundaryProps) {
  return (
    <QueryErrorResetBoundary>
      {({ reset }) => (
        <ErrorBoundary
          onReset={() => reset()}
          FallbackComponent={RecoverableErrorFallback}
          onError={onError}
        >
          {children}
        </ErrorBoundary>
      )}
    </QueryErrorResetBoundary>
  );
}
