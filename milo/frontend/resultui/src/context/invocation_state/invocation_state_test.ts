// Copyright 2021 The LUCI Authors.
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

import { assert } from 'chai';
import sinon from 'sinon';

import { QueryTestVariantsRequest, QueryTestVariantsResponse, TestVariant, TestVariantStatus, UISpecificService } from '../../services/resultdb';
import { AppState } from '../app_state/app_state';
import { InvocationState } from './invocation_state';

const variant1: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-1',
  variant: {'def': {'key1': 'val1'}},
  variantHash: 'key1:val1',
  status: TestVariantStatus.UNEXPECTED,
};

const variant2: TestVariant = {
  testId: 'invocation-a/test-suite-a/test-2',
  variant: {def: {'key1': 'val2'}},
  variantHash: 'key1:val2',
  status: TestVariantStatus.UNEXPECTED,
};

const variant3: TestVariant = {
  testId: 'invocation-a/test-suite-b/test-3',
  variant: {def: {'key1': 'val3'}},
  variantHash: 'key1:val3',
  status: TestVariantStatus.FLAKY,
};

const variant4: TestVariant = {
  testId: 'invocation-a/test-suite-B/test-4',
  variant: {'def': {'key1': 'val2'}},
  variantHash: 'key1:val2',
  status: TestVariantStatus.EXONERATED,
};

const variant5: TestVariant = {
  testId: 'invocation-a/test-suite-B/test-5',
  variant: {def: {'key1': 'val2', 'key2': 'val1'}},
  variantHash: 'key1:val2|key2:val1',
  status: TestVariantStatus.EXPECTED,
};


describe('InvocationState', () => {
  describe('filterVariant', async () => {
    const queryTestVariantsStub = sinon.stub<[QueryTestVariantsRequest], Promise<QueryTestVariantsResponse>>();
    queryTestVariantsStub.onCall(0).resolves({testVariants: [variant1, variant2, variant3, variant4, variant5]});

    const appState = {
      selectedTabId: '',
      uiSpecificService: {
        queryTestVariants: queryTestVariantsStub as typeof UISpecificService.prototype.queryTestVariants,
      },
    } as AppState;

    const invocationState = new InvocationState(appState);
    invocationState.invocationId = 'invocation-id';
    await invocationState.testLoader!.loadNextPage();
    invocationState.selectedNode = invocationState.testLoader!.node;
    after(() => invocationState.dispose());

    it('should not filter out anything when search text is empty', () => {
      invocationState.searchText = '';
      assert.deepEqual(invocationState.filteredUnexpectedVariants, [variant1, variant2]);
      assert.deepEqual(invocationState.filteredFlakyVariants, [variant3]);
      assert.deepEqual(invocationState.filteredExoneratedVariants, [variant4]);
      assert.deepEqual(invocationState.filteredExpectedVariants, [variant5]);
    });

    it('should filter out variants whose test ID doesn\'t match the search text', () => {
      invocationState.searchText = 'test-suite-a';
      assert.deepEqual(invocationState.filteredUnexpectedVariants, [variant1, variant2]);
      assert.deepEqual(invocationState.filteredFlakyVariants, []);
      assert.deepEqual(invocationState.filteredExoneratedVariants, []);
      assert.deepEqual(invocationState.filteredExpectedVariants, []);
    });

    it('search text should be case insensitive', () => {
      invocationState.searchText = 'test-suite-b';
      assert.deepEqual(invocationState.filteredUnexpectedVariants, []);
      assert.deepEqual(invocationState.filteredFlakyVariants, [variant3]);
      assert.deepEqual(invocationState.filteredExoneratedVariants, [variant4]);
      assert.deepEqual(invocationState.filteredExpectedVariants, [variant5]);
    });
  });
});
