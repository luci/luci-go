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

import { redirectionLoader } from './redirection_loader';

// react-router >= 7.10 added required `url` and `pattern` fields to
// LoaderFunctionArgs. `url` mirrors `request.url`, which is react-router's
// documented default, and `redirectionLoader` only reads `request`, so neither
// field changes what these tests exercise.
function loaderArgs(href: string) {
  const request = new Request(href);
  return {
    request,
    url: new URL(request.url),
    pattern: '/ui/bisection/*',
    params: {},
    context: '',
  };
}

describe('redirectionLoader', () => {
  describe('list table', () => {
    it('base page', () => {
      const response = redirectionLoader(
        loaderArgs('https://luci-milo-dev.appspot.com/ui/bisection'),
      );
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection',
      );
    });
    it('compile failure page', () => {
      const response = redirectionLoader(
        loaderArgs('https://luci-milo-dev.appspot.com/ui/bisection/analysis'),
      );
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection/compile-analysis',
      );
    });
    it('test failure page', () => {
      const response = redirectionLoader(
        loaderArgs(
          'https://luci-milo-dev.appspot.com/ui/bisection/test-analysis',
        ),
      );
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection/test-analysis',
      );
    });
  });
  describe('detail table', () => {
    it('compile failure page', () => {
      const response = redirectionLoader(
        loaderArgs(
          'https://luci-milo-dev.appspot.com/ui/bisection/analysis/b/123',
        ),
      );
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection/compile-analysis/b/123',
      );
    });
    it('test failure page', () => {
      const response = redirectionLoader(
        loaderArgs(
          'https://luci-milo-dev.appspot.com/ui/bisection/test-analysis/b/123',
        ),
      );
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection/test-analysis/b/123',
      );
    });
  });
});
