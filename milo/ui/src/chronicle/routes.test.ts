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
    const response = (await loader!({
      params: { workplanId: 'Lworkplan123:Nstage456' },
      request: new Request(
        'https://luci-milo.appspot.com/ui/chronicle/Lworkplan123:Nstage456/graph?param1=abc#hash',
      ),
      context: undefined,
    })) as Response;

    expect(response).toBeInstanceOf(Response);
    expect(response.status).toBe(302);
    expect(response.headers.get('Location')).toEqual(
      '/ui/chronicle/workplan123/graph?param1=abc&nodeId=Nstage456#hash',
    );
  });

  it('should redirect workplan ID starting with L', async () => {
    const response = (await loader!({
      params: { workplanId: 'Lworkplan123' },
      request: new Request(
        'https://luci-milo.appspot.com/ui/chronicle/Lworkplan123/graph?param1=abc',
      ),
      context: undefined,
    })) as Response;

    expect(response).toBeInstanceOf(Response);
    expect(response.status).toBe(302);
    expect(response.headers.get('Location')).toEqual(
      '/ui/chronicle/workplan123/graph?param1=abc',
    );
  });

  it('should redirect percent-encoded full node ID correctly', async () => {
    const response = (await loader!({
      params: { workplanId: 'L85400030111500308:S$init' },
      request: new Request(
        'https://luci-milo.appspot.com/ui/chronicle/L85400030111500308%3AS%24init/graph?param1=abc#hash',
      ),
      context: undefined,
    })) as Response;

    expect(response).toBeInstanceOf(Response);
    expect(response.status).toBe(302);
    expect(response.headers.get('Location')).toEqual(
      '/ui/chronicle/85400030111500308/graph?param1=abc&nodeId=S%24init#hash',
    );
  });

  it('should redirect StageAttempt full node ID correctly', async () => {
    const response = (await loader!({
      params: { workplanId: 'L85400030111500308:S$init:A1' },
      request: new Request(
        'https://luci-milo.appspot.com/ui/chronicle/L85400030111500308%3AS%24init%3AA1/graph?param1=abc#hash',
      ),
      context: undefined,
    })) as Response;

    expect(response).toBeInstanceOf(Response);
    expect(response.status).toBe(302);
    expect(response.headers.get('Location')).toEqual(
      '/ui/chronicle/85400030111500308/graph?param1=abc&nodeId=S%24init%3AA1#hash',
    );
  });

  it('should not redirect standard workplan ID', async () => {
    const response = await loader!({
      params: { workplanId: '12345' },
      request: new Request(
        'https://luci-milo.appspot.com/ui/chronicle/12345/graph?param1=abc',
      ),
      context: undefined,
    });

    expect(response).toBeNull();
  });
});
