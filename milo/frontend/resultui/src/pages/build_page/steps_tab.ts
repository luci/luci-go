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
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';

import '../../components/build_step_entry';
import '../../components/dot_spinner';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { BuildStatus } from '../../services/buildbucket';

export class StepsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  // TODO(crbug/1123362): save the setting.
  @observable.ref showPassed = true;
  @observable.ref showDebugLogs = false;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'steps';
  }

  @computed private get loaded() {
    return this.buildState.buildPageData !== null;
  }

  @computed private get noDisplayedStep() {
    if (this.showPassed) {
      return !this.buildState.buildPageData?.steps.length;
    }
    return !this.buildState.buildPageData?.steps.find((s) => s.status !== BuildStatus.Success);
  }

  protected render() {
    // TODO(crbug/1123362): add expand/collapse all buttons.
    return html`
      <div id="header">
        <div class="filters-container">
          Steps:
          <div class="filter">
            <input
              id="passed"
              type="checkbox"
              ?checked=${this.showPassed}
              @change=${(e: MouseEvent) => this.showPassed = (e.target as HTMLInputElement).checked}
            >
            <label for="passed" style="color: #33ac71;">Passed</label>
          </div class="filter">
          <div class="filter">
            <input id="others" type="checkbox" disabled checked>
            <label for="others">Others</label>
          </div>
        </div>
        <div class="filters-container-delimiter"></div>
        <div class="filters-container">
          Logs:
          <div class="filter">
            <input
              id="debug-logs-filter"
              type="checkbox"
              ?checked=${this.showDebugLogs}
              @change=${(e: MouseEvent) => this.showDebugLogs = (e.target as HTMLInputElement).checked}
            >
            <label for="debug-logs-filter">Debug</label>
          </div class="filter">
        </div>
      </div>
      <div id="main">
        ${this.buildState.buildPageData?.steps.map((step, i) => html`
        <milo-build-step-entry
          style=${styleMap({'display': step.status !== BuildStatus.Success || this.showPassed ? '' : 'none'})}
          class="list-entry"
          .expanded=${true}
          .number=${i + 1}
          .step=${step}
          .showDebugLogs=${this.showDebugLogs}
        ></milo-build-step-entry>
        `) || ''}
        <div
          class="list-entry"
          style=${styleMap({'display': this.loaded && this.noDisplayedStep ? '' : 'none'})}
        >
          No ${this.showPassed ? '' : 'failed'} steps.
        </div>
        <div id="load" class="list-entry" style=${styleMap({display: this.loaded ? 'none' : ''})}>
          Loading <milo-dot-spinner></milo-dot-spinner>
        </div>
      </div>
    `;
  }

  static styles = css`
    #header {
      display: grid;
      grid-template-columns: auto auto 1fr;
      grid-gap: 5px;
      height: 28px;
      padding: 5px 10px 3px 10px;
    }

    .filters-container {
      display: inline-block;
      padding: 0 5px;
      padding-top: 5px;
    }
    .filter {
      display: inline-block;
      margin: 0 5px;
    }
    .filter:last-child {
      margin-right: 0px;
    }
    .filters-container-delimiter {
      border-left: 1px solid #DDDDDD;
      width: 0px;
      height: 100%;
    }

    #main {
      padding-left: 10px;
      border-top: 1px solid #DDDDDD;
    }

    .list-entry {
      margin-top: 5px;
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
