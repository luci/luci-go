// Copyright 2020 The LUCI Authors.
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


import { computed, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import { AppState } from '../../context/app_state_provider';
import { consumeContext, provideContext } from '../../libs/context';
import { streamTestExonerations, streamTestResults, streamTests, TestLoader } from '../../models/test_loader';
import { ReadonlyTest, TestNode } from '../../models/test_node';
import { Expectancy, Invocation } from '../../services/resultdb';

/**
 * Records state of the invocation page.
 */
export class InvocationPageState {
  @observable.ref appState!: AppState;
  @observable.ref invocationId = '';
  @observable.ref selectedTabId = '';

  @computed
  get invocationName(): string {
    return 'invocations/' + this.invocationId;
  }

  @computed
  get invocationReq(): IPromiseBasedObservable<Invocation> {
    if (!this.appState.resultDb) {
      // Returns a promise that never resolves when resultDb isn't ready.
      return fromPromise(new Promise(() => {}));
    }
    return fromPromise(this.appState.resultDb.getInvocation({name: this.invocationName}));
  }

  @computed
  get invocation(): Invocation | null {
    if (this.invocationReq.state !== 'fulfilled') {
      return null;
    }
    return this.invocationReq.value;
  }

  @observable.ref selectedNode!: TestNode;
  @observable.ref showExpected = false;
  @observable.ref showExonerated = false;

  @computed
  private get testIter(): AsyncIterableIterator<ReadonlyTest> {
    if (!this.appState?.resultDb) {
      return (async function*() {})();
    }
    return streamTests(
      streamTestResults(
        {
          invocations: [this.invocationName],
          predicate: {
            expectancy: this.showExpected ? Expectancy.All : Expectancy.VariantsWithUnexpectedResults,
          },
        },
        this.appState.resultDb,
      ),
      this.showExonerated ?
        streamTestExonerations({invocations: [this.invocationName]}, this.appState.resultDb) :
        (async function*() {})(),
    );
  }

  @computed({keepAlive: true})
  get testLoader() { return new TestLoader(TestNode.newRoot(), this.testIter); }
}

export const consumePageState = consumeContext<'pageState', InvocationPageState>('pageState');
export const providePageState = provideContext<'pageState', InvocationPageState>('pageState');
