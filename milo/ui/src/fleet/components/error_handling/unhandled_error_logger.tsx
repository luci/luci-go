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

import { ReactNode, useEffect } from 'react';

import { logging } from '@/common/tools/logging';
import { useLogger } from '@/fleet/hooks/logger';
import { LogFrontendRequest_Severity } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export interface UnhandledErrorLoggerProps {
  readonly children: ReactNode;
}

/**
 * Registers logger for unhandled errors
 *
 * React **Error Boundaries** catch synchronous errors during rendering, lifecycle methods, or constructors.
 * However, errors in **asynchronous code** (e.g., Promises, setTimeout) or directly within **event handlers**
 * fall outside their scope. For these, global handlers like `window.onerror` (for general uncaught errors)
 * and `window.onunhandledrejection` (for unhandled Promise rejections) will catch them instead.
 */
export function UnhandledErrorLogger({ children }: UnhandledErrorLoggerProps) {
  const { logger } = useLogger();

  useEffect(() => {
    const handleError = async (event: ErrorEvent) => {
      try {
        await logger({
          message: event.message,
          source: event.filename,
          lineno: event.lineno,
          colno: event.colno,
          stack:
            event.error instanceof Error && event.error.stack
              ? event.error.stack
              : '',
          componentStack: '',
          url: window.location.href,
          severity: LogFrontendRequest_Severity.ERROR,
        });
      } catch (e) {
        logging.error(
          'CRITICAL: Error occurred within the handleError logger!',
        );
      }
    };

    const handleRejection = async (event: PromiseRejectionEvent) => {
      try {
        let message: string;
        let stack: string = '';

        if (event.reason instanceof Error) {
          message = event.reason.message;
          stack = event.reason.stack ?? '';
        } else if (typeof event.reason === 'string') {
          message = event.reason;
        } else {
          // Fallback for other types of rejections (e.g., plain objects, numbers)
          try {
            message = JSON.stringify(event.reason);
          } catch (e) {
            message = 'Unhandled promise rejection (unserializable reason)';
          }
        }

        await logger({
          message: message,
          source: '',
          lineno: -1,
          colno: -1,
          stack: stack,
          componentStack: '',
          url: window.location.href,
          severity: LogFrontendRequest_Severity.ERROR,
        });
      } catch (e) {
        logging.error(
          'CRITICAL: Error occurred within the handleRejection logger!',
        );
      }
    };

    window.addEventListener('error', handleError);
    window.addEventListener('unhandledrejection', handleRejection);

    return () => {
      window.removeEventListener('error', handleError);
      window.removeEventListener('unhandledrejection', handleRejection);
    };
  }, [logger]);

  return <>{children}</>;
}
