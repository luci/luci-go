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
import { SourceMapConsumer } from 'source-map';

import { errorReporter } from './reporter';

declare module 'source-map' {
  export interface SourceMapConsumerConstructor {
    initialize(mappings: { 'lib/mappings.wasm': string | ArrayBuffer }): void;
  }
}

/**
 * Registers global error handlers to capture uncaught exceptions and
 * unhandled promise rejections.
 *
 * @param reporter - A callback function that takes the captured
 * error or rejection reason and sends it to a reporting service.
 */
function registerErrorHandlers(reporter: (err: Error) => void) {
  window.addEventListener('error', (event: ErrorEvent) => {
    if (event.error instanceof Error) {
      reporter(event.error);
    } else if (event.error) {
      // This handles DOMExceptions and other values while creating a useful stack.
      reporter(
        new Error(`Non-error value thrown: ${JSON.stringify(event.error)}`),
      );
    } else {
      // Fallback for legacy events where event.error is not available.
      reporter(
        new Error(
          `${event.message} at ${event.filename}:${event.lineno}:${event.colno}`,
        ),
      );
    }
  });

  window.addEventListener(
    'unhandledrejection',
    (event: PromiseRejectionEvent) => {
      if (event) {
        if (event.reason instanceof Error) {
          reporter(event.reason);
        } else {
          // If not, create a new error to get a stack trace.
          // The message includes the original reason.
          // Note: The stack trace will originate from this line,
          // not from the original rejection point.
          reporter(
            new Error(
              `Unhandled promise rejection: ${JSON.stringify(event.reason)}`,
            ),
          );
        }
      }
    },
  );

  // Initialize SourceMapConsumer
  // For more info: https://github.com/mozilla/source-map?tab=readme-ov-file#use-on-the-web
  SourceMapConsumer.initialize({
    'lib/mappings.wasm': 'https://unpkg.com/source-map@0.7.6/lib/mappings.wasm',
  });
}

registerErrorHandlers(errorReporter.report);
