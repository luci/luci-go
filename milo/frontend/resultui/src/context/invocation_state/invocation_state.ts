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
import { streamTestBatches, streamTestExonerationBatches, streamTestResultBatches, streamVariantBatches, TestLoader } from '../../models/test_loader';
import { TestNode, VariantStatus } from '../../models/test_node';
import { Artifact, Expectancy, Invocation, QueryArtifactsResponse } from '../../services/resultdb';
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

  @computed
  private get invArtifactsRes(): IPromiseBasedObservable<QueryArtifactsResponse> {
    if (!this.appState.resultDb || !this.invocationName) {
      // Returns a promise that never resolves when resultDb isn't ready.
      return fromPromise(Promise.race([]));
    }
    const req = {
      invocations: [this.invocationName],
      followEdges: {includedInvocations: true, testResults: false},
    };

    // TODO(weiweilin): handle pagination.
    return fromPromise(this.appState.resultDb.queryArtifacts(req));
  }

  @computed({keepAlive: true})
  get invArtifacts(): Array<{invId: string, artifact: Artifact}> {
    if (this.invArtifactsRes.state !== FULFILLED) {
      return [];
    }
    return this.invArtifactsRes.value.artifacts?.map((a) => ({
      invId: /^invocations\/(?<inv_id>.+)\/artifacts\/.+$/.exec(a.name)!.groups!['inv_id'],
      artifact: a,
    })) || [];
  }

  @observable.ref selectedNode!: TestNode;

  @computed private get testResultBatchIterFn() {
    if (!this.appState.resultDb || !this.invocationName) {
      return async function*() { yield Promise.race([]); };
    }
    return iter.teeAsync(streamTestResultBatches(
      {
        invocations: [this.invocationName],
        predicate: {
          expectancy: this.userConfigs.tests.showExpectedVariant ? Expectancy.All : Expectancy.VariantsWithUnexpectedResults,
        },
        readMask: '*',
      },
      this.appState.resultDb,
    ));
  }

  @computed private get testExonerationBatchIterFn() {
    if (!this.appState.resultDb || !this.invocationName) {
      return async function*() {};
    }
    return iter.teeAsync(streamTestExonerationBatches(
      {invocations: [this.invocationName]},
      this.appState.resultDb,
    ));
  }

  @computed
  private get testIterFn() {
    let variantBatches = streamVariantBatches(
      this.testResultBatchIterFn(),
      this.testExonerationBatchIterFn(),
    );

    variantBatches = this.userConfigs.tests.showExoneratedVariant ?
      variantBatches :
      iter.mapAsync(variantBatches, (batch) => batch.filter((v) => v.status !== VariantStatus.Exonerated));

    // Known Issue:
    // A variant's status may change after filtering from expected/unexpected to
    // flaky if a result with a different expected value is received in the next
    // batch. In that case, some flaky variants are not filtered out.
    // This should be a rare occurrence. Since usually, results of the same test
    // variant should be in the same batch.
    variantBatches = this.userConfigs.tests.showFlakyVariant ?
      variantBatches :
      iter.mapAsync(variantBatches, (batch) => batch.filter((v) => v.status !== VariantStatus.Flaky));

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
