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

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useLogger } from '@/fleet/hooks/logger';
import { LogFrontendRequest_Severity } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export interface RecoverableLoggerErrorBoundaryProps {
  readonly children: ReactNode;
}

/**
 * Extends RecoverableErrorBoundary further to also log the error
 */
export function RecoverableLoggerErrorBoundary({
  children,
}: RecoverableLoggerErrorBoundaryProps) {
  const { logger } = useLogger();

  const handleError = (error: Error, info: ErrorInfo) => {
    logger({
      message: error.message,
      source: '',
      lineno: -1,
      colno: -1,
      stack: error.stack ?? '',
      componentStack: info.componentStack ?? '',
      url: window.location.href,
      severity: LogFrontendRequest_Severity.ERROR,
    });
  };

  return (
    <RecoverableErrorBoundary onError={handleError}>
      {children}
    </RecoverableErrorBoundary>
  );
}
