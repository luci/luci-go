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

import { cleanup, render, screen, waitFor } from '@testing-library/react';

import { Project } from '@/proto/go.chromium.org/luci/milo/proto/projectconfig/project.pb';
import { MiloInternalClientImpl } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import {
  QueryTestMetadataResponse,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestMetadataDetail } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_metadata.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestIdLabel } from './test_id_label';

describe('TestIdLabel', () => {
  afterEach(() => {
    cleanup();
  });

  const testSchema = 'testSchema';
  const baseTestMetadata = TestMetadataDetail.fromPartial({
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
  });
  const renderTestIdLabel = () => {
    render(
      <FakeContextProvider>
        <TestIdLabel project="testproject" testId="testid" />
      </FakeContextProvider>,
    );
    expect(screen.queryByText('testproject')).not.toBeNull();
    expect(screen.queryByText('testid')).not.toBeNull();
  };

  it('should load test source', async () => {
    const testMetadataStub = jest.spyOn(
      ResultDBClientImpl.prototype,
      'QueryTestMetadata',
    );
    testMetadataStub.mockResolvedValueOnce(
      QueryTestMetadataResponse.fromPartial({
        testMetadata: [baseTestMetadata],
      }),
    );

    renderTestIdLabel();
    expect(await screen.findByText('fakename')).not.toBeNull();
    const expectedSource =
      'https://chromium.googlesource.com/chromium/src/+/refs/heads/main/testfile#440';
    expect(
      (await screen.findByText('fakename')).getAttribute('href'),
    ).toStrictEqual(expectedSource);
  });

  it('should not crash if no test metadata returned from the query', async () => {
    const testMetadataStub = jest.spyOn(
      ResultDBClientImpl.prototype,
      'QueryTestMetadata',
    );
    testMetadataStub.mockResolvedValueOnce(
      QueryTestMetadataResponse.fromPartial({}),
    );

    expect(renderTestIdLabel).not.toThrow();
  });

  it('should load key value metadata using the metadataConfig', async () => {
    const testProjectCfg = Project.fromPartial({
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
    });
    const cfgStub = jest.spyOn(
      MiloInternalClientImpl.prototype,
      'GetProjectCfg',
    );
    cfgStub.mockResolvedValueOnce(testProjectCfg);
    const testMetadataStub = jest.spyOn(
      ResultDBClientImpl.prototype,
      'QueryTestMetadata',
    );
    testMetadataStub.mockResolvedValueOnce(
      QueryTestMetadataResponse.fromPartial({
        testMetadata: [baseTestMetadata],
      }),
    );

    renderTestIdLabel();
    await waitFor(() => expect(screen.queryByText('validPath')).not.toBeNull());
    expect(screen.queryByText('test case id')).not.toBeNull();
    expect(screen.queryByText('invalidPath1')).toBeNull();
    expect(screen.queryByText('invalidPath1')).toBeNull();
    expect(screen.queryByText('notString')).toBeNull();
  });
});
