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

import { expect, fixture, fixtureCleanup, html } from '@open-wc/testing/index-no-side-effects';
import * as sinon from 'sinon';

import './test_variant_entry';
import { AppState } from '../../../context/app_state';
import { ANONYMOUS_IDENTITY } from '../../../services/milo_internal';
import { TestResultBundle, TestStatus, TestVariant, TestVariantStatus } from '../../../services/resultdb';
import { Cluster } from '../../../services/weetbix';
import { TestVariantEntryElement } from './test_variant_entry';

const clusteringVersion = { algorithmsVersion: '1', rulesVersion: '1', configVersion: '1' };

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

describe('test_variant_entry_test', () => {
  afterEach(fixtureCleanup);

  it('should only query the necessary clusters', async () => {
    const appState = new AppState();
    appState.authState = { identity: ANONYMOUS_IDENTITY };
    const clusterStub = sinon.stub(appState.clustersService!, 'cluster');
    clusterStub.onCall(0).resolves({
      clusteringVersion,
      clusteredTestResults: [
        { clusters: [cluster1, cluster2, cluster3] },
        { clusters: [cluster1, cluster2, cluster3] },
      ],
    });

    function makeResult(resultId: string, expected: boolean, status: TestStatus, failureMsg: string): TestResultBundle {
      const ret: DeepMutable<TestResultBundle> = {
        result: {
          name: 'invocations/inv/test/test-id/result/' + resultId,
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
      <milo-test-variant-entry
        .appState=${appState}
        .invState=${{ project: 'proj' }}
        .variant=${tv}
      ></milo-test-variant-entry>
    `);

    expect(clusterStub.callCount).to.eq(1);
    expect(clusterStub.getCall(0).args).to.deep.eq([
      {
        project: 'proj',
        testResults: [
          { testId: 'test-id', failureReason: { primaryErrorMessage: 'reason3' } },
          { testId: 'test-id', failureReason: undefined },
        ],
      },
      { maxPendingMs: 1000 },
    ]);
  });
});
