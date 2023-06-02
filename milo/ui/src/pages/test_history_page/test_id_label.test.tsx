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

import { afterEach, beforeEach, expect, jest } from '@jest/globals';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { destroy, Instance } from 'mobx-state-tree';

import { AuthStateProvider } from '@/common/components/auth_state_provider';
import { ANONYMOUS_IDENTITY } from '@/common/libs/auth_state';
import {
  QueryTestMetadataResponse,
  ResultDb,
} from '@/common/services/resultdb';
import { Store, StoreProvider } from '@/common/store';

import { TestIdLabel } from './test_id_label';

describe('TestIdLabel', () => {
  let store: Instance<typeof Store>;
  let client: QueryClient;
  beforeEach(() => {
    client = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });
    store = Store.create({
      authState: { value: { identity: ANONYMOUS_IDENTITY } },
    });
  });
  afterEach(() => {
    cleanup();
    destroy(store);
  });

  it('should load test source', async () => {
    const tm: QueryTestMetadataResponse = {
      testMetadata: [
        {
          name: 'project/refhash',
          project: 'chromium',
          testId: 'fakeid',
          refHash: 'fakeHash',
          sourceRef: {
            gitiles: {
              host: 'chromium.googlesource.com',
              project: 'chromium/src',
              ref: 'refs/heads/main',
            },
          },
          testMetadata: {
            name: 'fakename',
            location: {
              repo: 'https://chromium.googlesource.com/chromium/src',
              fileName: '//testfile',
              line: 440,
            },
          },
        },
      ],
    };
    const testMetadataStub = jest.spyOn(
      ResultDb.prototype,
      'queryTestMetadata'
    );
    testMetadataStub.mockResolvedValueOnce(tm);

    render(
      <QueryClientProvider client={client}>
        <StoreProvider value={store}>
          <AuthStateProvider
            initialValue={{
              identity: 'identity-1',
              idToken: 'id-token-1',
              accessToken: 'access-token-1',
            }}
          >
            <TestIdLabel projectOrRealm="testrealm" testId="testid" />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>
    );
    expect(screen.queryByText('testrealm')).not.toBeNull();
    expect(screen.queryByText('testid')).not.toBeNull();
    expect(testMetadataStub.mock.calls.length).toStrictEqual(1);
    expect(await screen.findByText('fakename')).not.toBeNull();
    const expectedSource =
      'https://chromium.googlesource.com/chromium/src/+/refs/heads/main/testfile#440';
    expect(
      (await screen.findByText('fakename')).getAttribute('href')
    ).toStrictEqual(expectedSource);
  });
});
