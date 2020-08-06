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

import { MobxLitElement } from '@adobe/lit-mobx';
import { customElement, html } from 'lit-element';
import { AppState, consumeAppState } from '../context/app_state/app_state';
import { observable, computed} from 'mobx';
import { fromPromise, FULFILLED, IPromiseBasedObservable } from 'mobx-utils';
import { BuildState, consumeBuildState } from '../context/build_state/build_state';
import { RelatedBuildsData } from "../services/build_page"

export class RelatedBuildsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  @computed
  get relatedBuildsDataReq(): IPromiseBasedObservable<RelatedBuildsData> {
    if (!this.appState.buildPageService || !this.buildState.buildPageData) {
      return fromPromise(new Promise(() => {}));
    }
    return fromPromise(this.appState.buildPageService.getRelatedBuilds(this.buildState.buildPageData!.id));
  }

  @computed
  get relatedBuildsData(): RelatedBuildsData | null {
    if (this.relatedBuildsDataReq.state !== FULFILLED) {
      return null;
    }
    return this.relatedBuildsDataReq.value;
  }

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'related-builds';
  }

  protected render() {
    return html`
      There are ${this.relatedBuildsData?.related_builds.length} related builds
    `;
  }
}

customElement('tr-related-builds-tab')(
  consumeBuildState(
    consumeAppState(RelatedBuildsTabElement),
  )
);
