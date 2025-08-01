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
import { unminifyError } from './unminify_stackframe';

const baseAPIUrl =
  'https://clouderrorreporting.googleapis.com/v1beta1/projects/';

const noop = () => {};

type ErrorReporterOptions = {
  apiKey: string;
  projectId: string;
};

class ErrorReporter {
  /** The API key for the Google Cloud project. */
  private readonly apiKey: string;
  /** The ID of the Google Cloud project. */
  private readonly projectId: string;

  constructor({ apiKey, projectId }: ErrorReporterOptions) {
    this.apiKey = apiKey;
    this.projectId = projectId;
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
        service: 'web',
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
   * and sends it to the reporting service.
   * @param {unknown} err - The error to be reported. Can be any type.
   */
  report = (err: Error) => {
    unminifyError(err)
      .then((unminifiedError) => {
        this.send(unminifiedError).catch(noop);
      })
      .catch((_) => {
        this.send(String(err)).catch(noop);
      });
  };
}

export const errorReporter = [
  'luci-milo.appspot.com',
  'ci.chromium.org',
].includes(window.location.hostname)
  ? new ErrorReporter({
      apiKey: 'AIzaSyDxVV8kLK8CozsA1iKiPx6OjukSKQKmVbY',
      projectId: 'luci-milo',
    })
  : new ErrorReporter({
      apiKey: 'AIzaSyAyY1lwrHvFsIUrxyTuUDZZF1xTF6GbY08',
      projectId: 'luci-milo-dev',
    });
