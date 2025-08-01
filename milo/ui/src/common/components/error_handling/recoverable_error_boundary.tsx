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

import { errorReporter } from '@/common/api/error_reporting';

import { RecoverableErrorFallback } from './recoverable_error_fallback';

export interface RecoverableErrorBoundaryProps {
  readonly children: ReactNode;
}

/**
 * An error boundary that supports various error recovering strategies.
 */
// See https://tanstack.com/query/latest/docs/react/reference/QueryErrorResetBoundary
export function RecoverableErrorBoundary({
  children,
}: RecoverableErrorBoundaryProps) {
  return (
    <QueryErrorResetBoundary>
      {({ reset }) => (
        <ErrorBoundary
          onReset={() => reset()}
          FallbackComponent={RecoverableErrorFallback}
          onError={(err, _) => {
            errorReporter.report(err);
          }}
        >
          {children}
        </ErrorBoundary>
      )}
    </QueryErrorResetBoundary>
  );
}
