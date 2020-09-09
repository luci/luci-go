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
import { css, customElement, html } from 'lit-element';
import { observable } from 'mobx';

import '../../components/build_step_entry';
import '../../components/dot_spinner';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';

export class StepsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'steps';
  }

  protected render() {
    // TODO(crbug/1123362): add filters and expand/collapse all buttons.
    return html`
      ${this.buildState.buildPageData?.steps.map((step, i) => html`
      <milo-build-step-entry .expanded=${true} .number=${i + 1} .step=${step}></milo-build-step-entry>
      `) || html`<span id="load">Loading <milo-dot-spinner></milo-dot-spinner></span>`}
    `;
  }

  static styles = css`
    :host {
      padding-left: 10px;
    }

    #load {
      color: blue;
    }
  `;
}

customElement('milo-steps-tab')(
  consumeBuildState(
    consumeAppState(StepsTabElement),
  ),
);
