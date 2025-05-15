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

describe('redirectionLoader', () => {
  describe('list table', () => {
    it('base page', () => {
      const response = redirectionLoader({
        request: new Request('https://luci-milo-dev.appspot.com/ui/bisection'),
        params: {},
        context: '',
      });
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection',
      );
    });
    it('compile failure page', () => {
      const response = redirectionLoader({
        request: new Request(
          'https://luci-milo-dev.appspot.com/ui/bisection/analysis',
        ),
        params: {},
        context: '',
      });
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection/compile-analysis',
      );
    });
    it('test failure page', () => {
      const response = redirectionLoader({
        request: new Request(
          'https://luci-milo-dev.appspot.com/ui/bisection/test-analysis',
        ),
        params: {},
        context: '',
      });
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection/test-analysis',
      );
    });
  });
  describe('detail table', () => {
    it('compile failure page', () => {
      const response = redirectionLoader({
        request: new Request(
          'https://luci-milo-dev.appspot.com/ui/bisection/analysis/b/123',
        ),
        params: {},
        context: '',
      });
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection/compile-analysis/b/123',
      );
    });
    it('test failure page', () => {
      const response = redirectionLoader({
        request: new Request(
          'https://luci-milo-dev.appspot.com/ui/bisection/test-analysis/b/123',
        ),
        params: {},
        context: '',
      });
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/bisection/test-analysis/b/123',
      );
    });
  });
});
