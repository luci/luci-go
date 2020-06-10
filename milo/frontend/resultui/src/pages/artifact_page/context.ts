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
import { fromPromise } from 'mobx-utils';

import { AppState } from '../../context/app_state_provider';
import { consumeContext, provideContext } from '../../libs/context';


export enum ArtifactType {
  /**
   * A text diff artifact.
   */
  TextDiff,
  /**
   * Unsupported artifact type.
   */
  Unsupported,
}

/**
 * Records state of the invocation page.
 */
export class ArtifactPageState {
  @observable.ref artifactName!: string;
  @observable.ref appState!: AppState;
  @observable.ref selectedTabId = '';

  @computed({keepAlive: true})
  get artifactRes() {
    if (!this.appState.resultDb) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.resultDb.getArtifacts({name: this.artifactName}));
  }
  @computed get artifact() {
    return this.artifactRes.state === 'fulfilled' ? this.artifactRes.value : null;
  }

  @computed get artifactType() {
    if (!this.artifact) {
      return null;
    }
    if (this.artifact.contentType === 'text/plain' && this.artifact.artifactId.match(/_diff$/)) {
      return ArtifactType.TextDiff;
    }
    return ArtifactType.Unsupported;
  }

  @computed({keepAlive: true})
  get contentRes() {
    if (!this.appState.resultDb || !this.artifact) {
      return fromPromise(Promise.race([]));
    }
    // TODO(weiweilin): handle refresh.
    return fromPromise(fetch(this.artifact.fetchUrl!).then((res) => res.text()));
  }
  @computed get content() {
    return this.contentRes.state === 'fulfilled' ? this.contentRes.value : '';
  }
}

export const consumePageState = consumeContext<'pageState', ArtifactPageState>('pageState');
export const providePageState = provideContext<'pageState', ArtifactPageState>('pageState');
