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
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { destroy, Instance } from 'mobx-state-tree';

import * as authStateLib from '@/common/api/auth_state';
import { AuthStateProvider } from '@/common/components/auth_state_provider';
import { MiloInternal, Project } from '@/common/services/milo_internal';
import { ResultDb } from '@/common/services/resultdb';
import { TestMetadataDetail } from '@/common/services/resultdb';
import { Store, StoreProvider } from '@/common/store';

import { TestIdLabel } from './test_id_label';

jest.mock('@/common/api/auth_state', () => {
  const actual = jest.requireActual(
    '@/common/api/auth_state'
  ) as typeof authStateLib;
  const mocked: typeof authStateLib = {
    ...actual,
    // Wraps `queryAuthState` in a mock so we can mock its implementation later.
    queryAuthState: jest.fn(actual.queryAuthState),
  };
  return mocked;
});

const AUTH_STATE = {
  identity: 'identity-1',
  idToken: 'id-token-1',
  accessToken: 'access-token-1',
};

describe('TestIdLabel', () => {
  let store: Instance<typeof Store>;
  let client: QueryClient;
  let queryAuthStateSpy: jest.Mock<typeof authStateLib.queryAuthState>;
  beforeEach(() => {
    client = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });
    store = Store.create();
    queryAuthStateSpy = authStateLib.queryAuthState as jest.Mock<
      typeof authStateLib.queryAuthState
    >;
    queryAuthStateSpy.mockResolvedValue(AUTH_STATE);
  });
  afterEach(() => {
    queryAuthStateSpy.mockRestore();
    cleanup();
    destroy(store);
  });

  const testSchema = 'testSchema';
  const baseTestMetadata: TestMetadataDetail = {
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
      properties: {
        testCase: { id: { value: 'test case id' } },
        owners: [{ email: 'test@gmail.com' }],
      },
      propertiesSchema: testSchema,
    },
  };
  const renderTestIdLabel = () => {
    render(
      <QueryClientProvider client={client}>
        <StoreProvider value={store}>
          <AuthStateProvider initialValue={AUTH_STATE}>
            <TestIdLabel projectOrRealm="testrealm" testId="testid" />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>
    );
    expect(screen.queryByText('testrealm')).not.toBeNull();
    expect(screen.queryByText('testid')).not.toBeNull();
  };

  test('should load test source', async () => {
    const testMetadataStub = jest.spyOn(
      ResultDb.prototype,
      'queryTestMetadata'
    );
    testMetadataStub.mockResolvedValueOnce({
      testMetadata: [baseTestMetadata],
    });

    renderTestIdLabel();
    expect(await screen.findByText('fakename')).not.toBeNull();
    const expectedSource =
      'https://chromium.googlesource.com/chromium/src/+/refs/heads/main/testfile#440';
    expect(
      (await screen.findByText('fakename')).getAttribute('href')
    ).toStrictEqual(expectedSource);
  });

  test('should not crash if no test metadata returned from the query', async () => {
    const testMetadataStub = jest.spyOn(
      ResultDb.prototype,
      'queryTestMetadata'
    );
    testMetadataStub.mockResolvedValueOnce({});

    expect(renderTestIdLabel).not.toThrow();
  });

  test('should load key value metadata using the metadataConfig', async () => {
    const testProjectCfg: Project = {
      metadataConfig: {
        testMetadataProperties: [
          {
            schema: testSchema,
            displayItems: [
              { displayName: 'invalidPath1', path: 'testCase.id.value.a' },
              { displayName: 'invalidPath2', path: 'testCase.id.a.a' },
              { displayName: 'notString', path: 'testCaseInfo.owners' },
              { displayName: 'validPath', path: 'testCase.id.value' },
            ],
          },
        ],
      },
    };
    const cfgStub = jest.spyOn(MiloInternal.prototype, 'getProjectCfg');
    cfgStub.mockResolvedValueOnce(testProjectCfg);
    const testMetadataStub = jest.spyOn(
      ResultDb.prototype,
      'queryTestMetadata'
    );
    testMetadataStub.mockResolvedValueOnce({
      testMetadata: [baseTestMetadata],
    });

    renderTestIdLabel();
    await waitFor(() => expect(screen.queryByText('validPath')).not.toBeNull());
    expect(screen.queryByText('test case id')).not.toBeNull();
    expect(screen.queryByText('invalidPath1')).toBeNull();
    expect(screen.queryByText('invalidPath1')).toBeNull();
    expect(screen.queryByText('notString')).toBeNull();
  });
});
