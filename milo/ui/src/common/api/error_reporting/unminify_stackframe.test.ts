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

// Mock global fetch. This is the only dependency that should be mocked.
global.fetch = jest.fn();

describe('unminifyError', () => {
  beforeEach(() => {
    // Reset modules to clear the module-level sourceMapConsumerCache between tests.
    // This is crucial for testing the caching mechanism.
    jest.resetModules();
    // Clear mock history between tests.
    (fetch as jest.Mock).mockClear();
  });

  // A sample source map for testing.
  const MOCK_SOURCE_MAP = {
    version: 3,
    file: 'not_found_page.js',
    sources: ['../../../src/core/pages/not_found_page.tsx'],
    sourcesContent: [
      `export function NotFoundPage() {
  return (
    <div css={{ margin: '8px 16px' }}>
      We could not find the page you were looking for.
    </div>
  );
}

export function Component() {
  return <NotFoundPage />;
}`,
    ],
    names: ['NotFoundPage', 'jsx', 'margin', 'Component'],
    mappings:
      '6DAAO,SAASA,GAAe,CAC7B,OACEC,EAAC,OAAI,IAAK,CAAEC,OAAQ,UAAA,EAAa,SAAA,mDAEjC,CAEJ,CAEO,SAASC,GAAY,CAC1B,SAAQH,EAAA,EAAY,CACtB',
  };

  it('should unminify a stack trace successfully', async () => {
    const mockError = new Error('Test error');
    mockError.stack =
      'Error: Test error\n    at n (http://localhost:8080/not_found_page.min.js:1:90)';

    (fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(MOCK_SOURCE_MAP),
    });

    const unminified = await unminifyError(mockError);

    expect(fetch).toHaveBeenCalledWith(
      'http://localhost:8080/not_found_page.min.js.map',
    );
    expect(unminified).toContain('Error: Test error');
    expect(unminified).toContain('[Host: localhost]');
    expect(unminified).toContain(
      'at NotFoundPage (src/core/pages/not_found_page.tsx:3:6)',
    );
  });
});
