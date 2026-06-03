// Copyright 2024 The LUCI Authors.
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

import { defineConfig } from 'cypress';

export default defineConfig({
  video: true,
  e2e: {
    baseUrl: 'http://localhost:8000',
    retries: 2,
    setupNodeEvents(on, config) {
      on('before:browser:launch', (browser, launchOptions) => {
        // Use an isolated remote debugging port to avoid conflicts with
        // other active Chrome instances (e.g. background agent web viewports).
        if (browser.family === 'chromium' && browser.name !== 'electron') {
          const debugPort = process.env.CYPRESS_REMOTE_DEBUGGING_PORT || '9333';
          const hasPort = launchOptions.args.some((arg) =>
            arg.startsWith('--remote-debugging-port='),
          );
          if (!hasPort) {
            launchOptions.args.push(`--remote-debugging-port=${debugPort}`);
          } else {
            launchOptions.args = launchOptions.args.map((arg) =>
              arg.startsWith('--remote-debugging-port=')
                ? `--remote-debugging-port=${debugPort}`
                : arg,
            );
          }
        }
        return launchOptions;
      });
      return config;
    },
  },
});
