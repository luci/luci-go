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
import { fromPromise, FULFILLED, IPromiseBasedObservable } from 'mobx-utils';

import { consumeContext, provideContext } from '../../libs/context';
import * as iter from '../../libs/iter_utils';
import { streamTestBatches, streamVariantBatches, TestLoader } from '../../models/test_loader';
import { TestNode } from '../../models/test_node';
import { Invocation, TestVariantStatus } from '../../services/resultdb';
import { AppState } from '../app_state/app_state';
import { UserConfigs } from '../app_state/user_configs';

/**
 * Records state of an invocation.
 */
export class InvocationState {
  @observable.ref invocationId = '';
  @observable.ref initialized = false;

  constructor(private appState: AppState, private userConfigs: UserConfigs) {}

  @observable.ref private isDisposed = false;

  /**
   * Perform cleanup.
   * Must be called before the object is GCed.
   */
  dispose() {
    this.isDisposed = true;

    // Evaluates @computed({keepAlive: true}) properties after this.isDisposed
    // is set to true so they no longer subscribes to any external observable.
    // tslint:disable: no-unused-expression
    this.testLoader;
    // tslint:enable: no-unused-expression
  }


  @computed
  get invocationName(): string | null {
    if (!this.invocationId) {
      return null;
    }
    return 'invocations/' + this.invocationId;
  }

  @computed
  get invocationRes(): IPromiseBasedObservable<Invocation> {
    if (!this.appState.resultDb || !this.invocationName) {
      // Returns a promise that never resolves when resultDb isn't ready.
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getInvocation({name: this.invocationName}));
  }

  @computed
  get invocation(): Invocation | null {
    if (this.invocationRes.state !== FULFILLED) {
      return null;
    }
    return this.invocationRes.value;
  }

  @observable.ref selectedNode!: TestNode;

  @computed private get testVariantBatchFn() {
    if (!this.appState.uiSpecificService || !this.invocationName) {
      return async function*() { yield Promise.race([]); };
    }

    return iter.teeAsync(streamVariantBatches(
      {
        invocations: [this.invocationName],
      },
      this.appState.uiSpecificService,
    ));
  }

  @computed
  private get testIterFn() {
    let variantBatches = this.testVariantBatchFn();

    variantBatches = this.userConfigs.tests.showExpectedVariant ?
      variantBatches :
      iter.mapAsync(variantBatches, (batch) => batch.filter((v) => v.status !== TestVariantStatus.EXPECTED));

    variantBatches = this.userConfigs.tests.showExoneratedVariant ?
      variantBatches :
      iter.mapAsync(variantBatches, (batch) => batch.filter((v) => v.status !== TestVariantStatus.EXONERATED));

    variantBatches = this.userConfigs.tests.showFlakyVariant ?
      variantBatches :
      iter.mapAsync(variantBatches, (batch) => batch.filter((v) => v.status !== TestVariantStatus.FLAKY));

    return iter.teeAsync(streamTestBatches(variantBatches));
  }

  @computed({keepAlive: true})
  get testLoader(): TestLoader {
    if (this.isDisposed) {
      return new TestLoader(TestNode.newRoot(), (async function*() {})());
    }
    return new TestLoader(TestNode.newRoot(), this.testIterFn());
  }
}

export const consumeInvocationState = consumeContext<'invocationState', InvocationState>('invocationState');
export const provideInvocationState = provideContext<'invocationState', InvocationState>('invocationState');
