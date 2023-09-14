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

import { searchRedirectionLoader } from './search_redirection_loader';

describe('searchLoader', () => {
  describe('Builder search', () => {
    it('should redirect to builder-search when t param is missing', () => {
      const response = searchRedirectionLoader({
        request: new Request(
          'https://luci-milo-dev.appspot.com/ui/search?q=query',
        ),
      });
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/builder-search?q=query',
      );
    });

    it('should redirect to builder-search when query param t is BUILDERS', () => {
      const response = searchRedirectionLoader({
        request: new Request(
          'https://luci-milo-dev.appspot.com/ui/search?q=query&t=BUILDERS',
        ),
      });
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/builder-search?q=query',
      );
    });
  });

  describe('Test search', () => {
    it('should redirect to test-search if t param is TESTS', () => {
      const response = searchRedirectionLoader({
        request: new Request(
          'https://luci-milo-dev.appspot.com/ui/search?q=query&t=TESTS&tp=chrome',
        ),
      });
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chrome/test-search?q=query',
      );
    });

    it('empty project for tests search should default to chromium', () => {
      const response = searchRedirectionLoader({
        request: new Request(
          'https://luci-milo-dev.appspot.com/ui/search?q=query&t=TESTS',
        ),
      });
      expect(response.status).toBe(302);
      expect(response.headers.get('Location')).toEqual(
        '/ui/p/chromium/test-search?q=query',
      );
    });
  });
});
