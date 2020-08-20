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
import { customElement, html, property } from 'lit-element';
import { autorun, observable, when } from 'mobx';
import { REJECTED } from 'mobx-utils';

import { AppState, consumeAppState } from '../app_state/app_state';
import { BuildState, provideBuildState } from './build_state';

/**
 * Provides buildState to be shared across the app.
 */
export class BuildStateProviderElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @property() buildState!: BuildState;

  private disposers: Array<() => void> = [];
  connectedCallback() {
    super.connectedCallback();
    this.disposers.push(autorun(
      () => this.buildState = new BuildState(this.appState),
    ));
    this.disposers.push(when(
      () => this.buildState.buildPageDataReq.state === REJECTED,
      () => this.dispatchEvent(new ErrorEvent('error', {
        message: this.buildState.buildPageDataReq.value.toString(),
        composed: true,
        bubbles: true,
      })),
    ));
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  protected render() {
    return html`
      <slot></slot>
    `;
  }
}

customElement('milo-build-state-provider')(
  provideBuildState(
    consumeAppState(BuildStateProviderElement),
  ),
);
