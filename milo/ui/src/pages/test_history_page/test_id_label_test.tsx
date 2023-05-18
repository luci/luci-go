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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { expect } from 'chai';
import { destroy, Instance } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { AuthStateProvider } from '../../components/auth_state_provider';
import { ANONYMOUS_IDENTITY } from '../../libs/auth_state';
import { QueryTestMetadataResponse, ResultDb } from '../../services/resultdb';
import { Store, StoreProvider } from '../../store';
import { TestIdLabel } from './test_id_label';

describe('TestIDLabel', () => {
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
    store = Store.create({ authState: { value: { identity: ANONYMOUS_IDENTITY } } });
  });
  afterEach(() => destroy(store));

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
    const testMetadataStub = sinon.stub(ResultDb.prototype, 'queryTestMetadata');
    testMetadataStub.onCall(0).resolves(tm);

    render(
      <QueryClientProvider client={client}>
        <StoreProvider value={store}>
          <AuthStateProvider
            initialValue={{ identity: 'identity-1', idToken: 'id-token-1', accessToken: 'access-token-1' }}
          >
            <TestIdLabel projectOrRealm="testrealm" testId="testid" />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>
    );
    expect(screen.queryByText('testrealm')).to.not.be.null;
    expect(screen.queryByText('testid')).to.not.be.null;
    expect(testMetadataStub.callCount).to.eq(1);
    expect(await screen.findByText('fakename')).to.not.be.null;
    const expectedSource = 'https://chromium.googlesource.com/chromium/src/+/refs/heads/main/testfile#440';
    expect((await screen.findByText('fakename')).getAttribute('href')).to.eq(expectedSource);
  });
});
