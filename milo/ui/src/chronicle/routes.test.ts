// Copyright 2026 The LUCI Authors.
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

import { chronicleRoutes } from './routes';

// react-router >= 7.10 added required `url` and `pattern` fields to
// LoaderFunctionArgs. `url` mirrors `request.url`, which is react-router's
// documented default, and the loader under test only reads `params` and
// `request`, so neither field changes what these tests exercise.
function loaderArgs(workplanId: string, href: string) {
  const request = new Request(href);
  return {
    params: { workplanId },
    request,
    url: new URL(request.url),
    pattern: ':workplanId',
    context: undefined,
  };
}

describe('chronicleRoutes redirect loader', () => {
  const canonicalRoute = chronicleRoutes.find((r) => r.path === ':workplanId');
  const loader =
    typeof canonicalRoute?.loader === 'function'
      ? canonicalRoute.loader
      : undefined;

  beforeAll(() => {
    if (!loader) {
      throw new Error('loader is not a function');
    }
  });

  it('should exist', () => {
    expect(loader).toBeDefined();
  });

  it('should redirect full node ID to workplan ID and query param', async () => {
    const response = (await loader!(
      loaderArgs(
        'Lworkplan123:Nstage456',
        'https://luci-milo.appspot.com/ui/chronicle/Lworkplan123:Nstage456/graph?param1=abc#hash',
      ),
    )) as Response;

    expect(response).toBeInstanceOf(Response);
    expect(response.status).toBe(302);
    expect(response.headers.get('Location')).toEqual(
      '/ui/chronicle/workplan123/graph?param1=abc&nodeId=Nstage456#hash',
    );
  });

  it('should redirect workplan ID starting with L', async () => {
    const response = (await loader!(
      loaderArgs(
        'Lworkplan123',
        'https://luci-milo.appspot.com/ui/chronicle/Lworkplan123/graph?param1=abc',
      ),
    )) as Response;

    expect(response).toBeInstanceOf(Response);
    expect(response.status).toBe(302);
    expect(response.headers.get('Location')).toEqual(
      '/ui/chronicle/workplan123/graph?param1=abc',
    );
  });

  it('should redirect percent-encoded full node ID correctly', async () => {
    const response = (await loader!(
      loaderArgs(
        'L85400030111500308:S$init',
        'https://luci-milo.appspot.com/ui/chronicle/L85400030111500308%3AS%24init/graph?param1=abc#hash',
      ),
    )) as Response;

    expect(response).toBeInstanceOf(Response);
    expect(response.status).toBe(302);
    expect(response.headers.get('Location')).toEqual(
      '/ui/chronicle/85400030111500308/graph?param1=abc&nodeId=S%24init#hash',
    );
  });

  it('should redirect StageAttempt full node ID correctly', async () => {
    const response = (await loader!(
      loaderArgs(
        'L85400030111500308:S$init:A1',
        'https://luci-milo.appspot.com/ui/chronicle/L85400030111500308%3AS%24init%3AA1/graph?param1=abc#hash',
      ),
    )) as Response;

    expect(response).toBeInstanceOf(Response);
    expect(response.status).toBe(302);
    expect(response.headers.get('Location')).toEqual(
      '/ui/chronicle/85400030111500308/graph?param1=abc&nodeId=S%24init%3AA1#hash',
    );
  });

  it('should not redirect standard workplan ID', async () => {
    const response = await loader!(
      loaderArgs(
        '12345',
        'https://luci-milo.appspot.com/ui/chronicle/12345/graph?param1=abc',
      ),
    );

    expect(response).toBeNull();
  });
});
