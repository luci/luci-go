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
import { logging } from '@/common/tools/logging';
import { timeout } from '@/generic_libs/tools/utils';

import { unminifyError } from './unminify_stackframe';
import { getFleetConsoleProject } from './utils';

const baseAPIUrl =
  'https://clouderrorreporting.googleapis.com/v1beta1/projects/';

const noop = () => {};

/**
 * Reports errors to a single Google Cloud project.
 */
class ErrorReporter {
  constructor(
    /** The API key for the Google Cloud project. */
    private readonly apiKey: string,
    /** The ID of the Google Cloud project. */
    private readonly projectId: string,
    /** A function that returns true if the error should be reported. */
    private readonly filter: (err: Error) => boolean = () => true,
  ) {
    if (!apiKey || !projectId) {
      logging.warn(
        'Error reporting is disabled due to missing apiKey or projectId.',
      );
    }
  }

  /**
   * Sends a formatted error report to the Cloud Error Reporting API.
   * @param {string} message - The error message to report.
   */
  private async send(message: string) {
    const reportUrl =
      baseAPIUrl + this.projectId + '/events:report?key=' + this.apiKey;

    const payload = {
      message: message,
      serviceContext: {
        service: 'ui',
      },
      context: {
        httpRequest: {
          userAgent: window.navigator.userAgent,
          url: window.location.href,
        },
      },
    };

    const response = await fetch(reportUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) throw new Error(`Failed to send: ${response.statusText}`);
  }

  /**
   * Normalizes the error, attempts to un-minify it,
   * and sends it to the reporting service if the filter passes.
   * @param {Error} err - The error to be reported.
   */
  report = (err: Error) => {
    if (!this.apiKey || !this.projectId || !this.filter(err)) {
      return;
    }

    // Sometimes the error happens quickly and some stuff is not loaded.
    // This timeout somehow prevents it.
    timeout(1000).then(() =>
      unminifyError(err)
        .then((unminifiedError) => {
          this.send(unminifiedError).catch(noop);
        })
        .catch((unminifyErr) => {
          this.send(
            String(err) +
              '\n--- Failed to unminify stack trace ---\n' +
              String(unminifyErr),
          ).catch(noop);
        }),
    );
  };
}

/**
 * A composite error reporter that can send errors to multiple reporters.
 */
class CompositeErrorReporter {
  constructor(private readonly reporters: ErrorReporter[]) {
    this.reporters = reporters;
  }

  report = (err: Error) => {
    for (const reporter of this.reporters) {
      reporter.report(err);
    }
  };
}

const { fleetConsoleProjectApiKey, fleetConsoleProjectId } =
  getFleetConsoleProject();

export const errorReporter = new CompositeErrorReporter([
  new ErrorReporter(SETTINGS.milo.errorReportingApiKey, SETTINGS.milo.project),
  // TODO(vaghinak): This is a temporary solution to redirect fleet console specific
  // errors to the fleet console cloud project until there is a better solution found.
  // If we end up keeping would be better to extract the apiKey and project to the configs
  // as we did with the milo ones
  new ErrorReporter(fleetConsoleProjectApiKey, fleetConsoleProjectId, (_) =>
    window.location.pathname.includes('ui/fleet'),
  ),
]);
