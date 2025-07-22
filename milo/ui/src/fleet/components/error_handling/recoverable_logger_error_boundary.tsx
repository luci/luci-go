// Copyright 2025 The LUCI Authors.
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

// Copyright 2025 The LUCI Authors.
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

import { ErrorInfo, ReactNode } from 'react';

import { errorHandler } from '@/common/api/stackdriver_errors';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';

export interface RecoverableLoggerErrorBoundaryProps {
  readonly children: ReactNode;
}

/**
 * Extends RecoverableErrorBoundary further to also log the error
 */
export function RecoverableLoggerErrorBoundary({
  children,
}: RecoverableLoggerErrorBoundaryProps) {
  // const { logger } = useLogger();

  const handleError = (error: Error, _: ErrorInfo) => {
    // // eslint-disable-next-line no-console
    // console.log('sending error to gcp:', error);

    errorHandler.report(error);
  };

  return (
    <RecoverableErrorBoundary onError={handleError}>
      {children}
    </RecoverableErrorBoundary>
  );
}
