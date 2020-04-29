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
import { Invocation } from '../../services/resultdb';

/**
 * Records state of the invocation page.
 */
export class InvocationPageState {
  @observable.ref appState = new AppState();
  @observable.ref invocationId = '';

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
}

export const consumePageState = consumeContext<'pageState', InvocationPageState>('pageState');
export const providePageState = provideContext<'pageState', InvocationPageState>('pageState');
