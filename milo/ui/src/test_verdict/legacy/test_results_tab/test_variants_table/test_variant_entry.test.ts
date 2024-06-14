// Copyright 2022 The LUCI Authors.
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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import {
  aTimeout,
  fixture,
  fixtureCleanup,
  html,
} from '@open-wc/testing-helpers';
import { LitElement } from 'lit';
import { customElement } from 'lit/decorators.js';
import { destroy } from 'mobx-state-tree';

import './test_variant_entry';
import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { Cluster } from '@/common/services/luci_analysis';
import {
  TestResultBundle,
  TestStatus,
  TestVariant,
  TestVariantStatus,
} from '@/common/services/resultdb';
import { provideStore, Store, StoreInstance } from '@/common/store';
import { logging } from '@/common/tools/logging';
import { ExpandableEntryElement } from '@/generic_libs/components/expandable_entry';
import { provider } from '@/generic_libs/tools/lit_context';
import {
  Notifier,
  provideNotifier,
} from '@/generic_libs/tools/observer_element';
import { DeepMutable } from '@/generic_libs/types';

import { provideInvId, provideProject, provideTestTabUrl } from './context';
import { TestVariantEntryElement } from './test_variant_entry';

jest.mock('@/test_verdict/components/result_entry', () => ({}));

const clusteringVersion = {
  algorithmsVersion: '1',
  rulesVersion: '1',
  configVersion: '1',
};

const cluster1: Cluster = {
  clusterId: {
    algorithm: 'rule',
    id: 'cluster1',
  },
  bug: {
    system: 'monorail',
    id: '1234',
    linkText: 'crbug.com/1234',
    url: 'http://crbug.com/1234',
  },
};

const cluster2: Cluster = {
  clusterId: {
    algorithm: 'rule',
    id: 'cluster2',
  },
  bug: {
    system: 'monorail',
    id: '5678',
    linkText: 'crbug.com/5678',
    url: 'http://crbug.com/5678',
  },
};

const cluster3: Cluster = {
  clusterId: {
    algorithm: 'rule',
    id: 'cluster2',
  },
  bug: {
    system: 'buganizer',
    id: '1234',
    linkText: 'b/1234',
    url: 'http://b/1234',
  },
};

@customElement('milo-test-variant-entry-test-context')
@provider
class TestVariantEntryTestContextElement extends LitElement {
  @provideStore()
  store!: StoreInstance;

  @provideNotifier()
  notifier: Notifier = {
    subscribe(ele) {
      ele.notify();
    },
    unsubscribe(_ele) {},
  };

  @provideProject()
  proj = 'proj';

  @provideInvId()
  inv = 'inv';

  @provideTestTabUrl()
  testTabUrl = 'https://test.com/test-results';

  protected render() {
    return html`<slot></slot>`;
  }
}

describe('<TestVariantEntry />', () => {
  let store: StoreInstance;
  let logErrorMock: jest.SpyInstance;

  beforeEach(() => {
    store = Store.create({
      authState: { value: { identity: ANONYMOUS_IDENTITY } },
    });
    logErrorMock = jest.spyOn(logging, 'error').mockImplementation(() => {});
  });
  afterEach(() => {
    fixtureCleanup();
    destroy(store);
    logErrorMock.mockRestore();
  });

  test('should only query the necessary clusters', async () => {
    const clusterStub = jest.spyOn(store.services.clusters!, 'cluster');
    clusterStub.mockResolvedValueOnce({
      clusteringVersion,
      clusteredTestResults: [
        { clusters: [cluster1, cluster2, cluster3] },
        { clusters: [cluster1, cluster2, cluster3] },
      ],
    });

    function makeResult(
      resultId: string,
      expected: boolean,
      status: TestStatus,
      failureMsg: string,
    ): TestResultBundle {
      const ret: DeepMutable<TestResultBundle> = {
        result: {
          name: 'invocations/inv/tests/test-id/results/' + resultId,
          testId: 'test-id',
          resultId,
          expected,
          status,
          summaryHtml: '',
          startTime: '2022-01-01',
        },
      };
      if (failureMsg) {
        ret.result.failureReason = { primaryErrorMessage: failureMsg };
      }
      return ret;
    }

    const tv: TestVariant = {
      testId: 'test-id',
      sourcesId: '1',
      variantHash: 'vhash',
      status: TestVariantStatus.FLAKY,
      results: [
        // Passed result. Ignore.
        makeResult('result-1', false, TestStatus.Pass, 'reason1'),
        // Skipped result. Ignore.
        makeResult('result-2', false, TestStatus.Skip, 'reason2'),
        // Failed result. Query cluster.
        makeResult('result-3', false, TestStatus.Fail, 'reason3'),
        // Failed result but expected. Ignored.
        makeResult('result-4', true, TestStatus.Fail, 'reason4'),
        // Abort result. Query Cluster.
        makeResult('result-5', false, TestStatus.Abort, ''),
      ],
    };

    await fixture<TestVariantEntryElement>(html`
      <milo-test-variant-entry-test-context .store=${store}>
        <milo-test-variant-entry
          .store=${store}
          .variant=${tv}
        ></milo-test-variant-entry>
      </milo-test-variant-entry-test-context>
    `);

    expect(clusterStub.mock.calls.length).toStrictEqual(1);
    expect(clusterStub.mock.lastCall).toEqual([
      {
        project: 'proj',
        testResults: [
          {
            testId: 'test-id',
            failureReason: { primaryErrorMessage: 'reason3' },
          },
          { testId: 'test-id', failureReason: undefined },
        ],
      },
      {},
      { maxPendingMs: 1000 },
    ]);
  });

  test('should work properly if failed to query clusters from luci-analysis', async () => {
    const clusterStub = jest.spyOn(store.services.clusters!, 'cluster');
    clusterStub.mockRejectedValueOnce(
      new GrpcError(RpcCode.PERMISSION_DENIED, 'not allowed'),
    );

    const tv: TestVariant = {
      testId: 'test-id',
      sourcesId: '1',
      variantHash: 'vhash',
      status: TestVariantStatus.UNEXPECTED,
      results: [
        {
          result: {
            name: 'invocations/inv/tests/test-id/results/result-id',
            testId: 'test-id',
            resultId: 'result-id',
            expected: false,
            status: TestStatus.Fail,
            summaryHtml: '',
            startTime: '2022-01-01',
          },
        },
      ],
    };

    const ele = await fixture<TestVariantEntryTestContextElement>(html`
      <milo-test-variant-entry-test-context .store=${store}>
        <milo-test-variant-entry .variant=${tv}></milo-test-variant-entry>
      </milo-test-variant-entry-test-context>
    `);
    const tvEntry = ele.querySelector<TestVariantEntryElement>(
      'milo-test-variant-entry',
    )!;
    await aTimeout(0);

    expect(clusterStub.mock.calls.length).toStrictEqual(1);
    expect(clusterStub.mock.lastCall).toEqual([
      {
        project: 'proj',
        testResults: [{ testId: 'test-id', failureReason: undefined }],
      },
      {},
      { maxPendingMs: 1000 },
    ]);

    // New interactions are still handled.
    tvEntry.expanded = true;
    await tvEntry.updateComplete;
    expect(
      tvEntry.shadowRoot!.querySelector<ExpandableEntryElement>(
        'milo-expandable-entry',
      )!.expanded,
    ).toBeTruthy();
  });

  test('should generate test URLs correctly', async () => {
    const tv: TestVariant = {
      testId: 'test-id',
      sourcesId: '1',
      variantHash: 'vhash',
      status: TestVariantStatus.UNEXPECTED,
      results: [
        {
          result: {
            name: 'invocations/inv/tests/test-id/results/result-id',
            testId: 'test-id',
            resultId: 'result-id',
            expected: false,
            status: TestStatus.Fail,
            summaryHtml: '',
            startTime: '2022-01-01',
          },
        },
      ],
    };

    const ele = await fixture<TestVariantEntryTestContextElement>(html`
      <milo-test-variant-entry-test-context .store=${store}>
        <milo-test-variant-entry .variant=${tv}></milo-test-variant-entry>
      </milo-test-variant-entry-test-context>
    `);
    const tvEntry = ele.querySelector<TestVariantEntryElement>(
      'milo-test-variant-entry',
    )!;
    expect(tvEntry.selfLink).toStrictEqual(
      'https://test.com/test-results?q=ExactID%3Atest-id+VHash%3Avhash',
    );
  });
});
