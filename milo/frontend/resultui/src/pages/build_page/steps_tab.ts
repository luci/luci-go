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
import '@material/mwc-button';
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';

import '../../components/build_step_entry';
import { BuildStepEntryElement } from '../../components/build_step_entry';
import '../../components/dot_spinner';
import '../../components/hotkey';
import '../../components/lazy_list';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { consumeUserConfigs, UserConfigs } from '../../context/app_state/user_configs';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { BuildStatus } from '../../services/buildbucket';

export class StepsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref userConfigs!: UserConfigs;
  @observable.ref buildState!: BuildState;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'steps';
  }

  @computed private get loaded() {
    return this.buildState.buildPageData !== null;
  }

  @computed private get noDisplayedStep() {
    if (this.userConfigs.steps.showSucceededSteps) {
      return !this.buildState.buildPageData?.steps?.length;
    }
    return !this.buildState.buildPageData?.steps?.find((s) => s.status !== BuildStatus.Success);
  }

  private allStepsWereExpanded = false;
  private toggleAllSteps(expand: boolean) {
    this.allStepsWereExpanded = expand;
    this.shadowRoot!.querySelectorAll<BuildStepEntryElement>('milo-build-step-entry')
      .forEach((e) => e.toggleAllSteps(expand));
  }
  toggleAllStepsByHotkey = () => this.toggleAllSteps(!this.allStepsWereExpanded);

  protected render() {
    return html`
      <div id="header">
        <div class="filters-container">
          Steps:
          <div class="filter">
            <input
              id="succeeded"
              type="checkbox"
              ?checked=${this.userConfigs.steps.showSucceededSteps}
              @change=${(e: MouseEvent) => this.userConfigs.steps.showSucceededSteps = (e.target as HTMLInputElement).checked}
            >
            <label for="succeeded" style="color: var(--success-color);">Succeeded</label>
          </div>
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
              ?checked=${this.userConfigs.steps.showDebugLogs}
              @change=${(e: MouseEvent) => this.userConfigs.steps.showDebugLogs = (e.target as HTMLInputElement).checked}
            >
            <label for="debug-logs-filter">Debug</label>
          </div>
        </div>
        <span></span>
        <milo-hotkey key="x" .handle=${this.toggleAllStepsByHotkey} title="press x to expand/collapse all entries">
          <mwc-button
            class="action-button"
            dense unelevated
            @click=${() => this.toggleAllSteps(true)}
          >Expand All</mwc-button>
          <mwc-button
            class="action-button"
            dense unelevated
            @click=${() => this.toggleAllSteps(false)}
          >Collapse All</mwc-button>
        </milo-hotkey>
      </div>
      <milo-lazy-list id="main" .growth=${300}>
        ${this.buildState.buildPageData?.steps?.map((step, i) => html`
        <milo-build-step-entry
          style=${styleMap({'display': step.status !== BuildStatus.Success || this.userConfigs.steps.showSucceededSteps ? '' : 'none'})}
          .expanded=${step.status !== BuildStatus.Success}
          .number=${i + 1}
          .step=${step}
          .prerender=${true}
        ></milo-build-step-entry>
        `) || ''}
        <div
          class="list-entry"
          style=${styleMap({'display': this.loaded && this.noDisplayedStep ? '' : 'none'})}
        >
          ${this.userConfigs.steps.showSucceededSteps ? 'No steps.' : 'All steps succeeded.'}
        </div>
        <div id="load" class="list-entry" style=${styleMap({display: this.loaded ? 'none' : ''})}>
          Loading <milo-dot-spinner></milo-dot-spinner>
        </div>
      </milo-lazy-list>
    `;
  }

  static styles = css`
    :host {
      display: grid;
      grid-template-rows: auto 1fr;
      overflow-y: hidden;
    }

    #header {
      display: grid;
      grid-template-columns: auto auto auto 1fr auto auto;
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
      border-left: 1px solid var(--divider-color);
      width: 0px;
      height: 100%;
    }

    #main {
      padding-top: 5px;
      padding-left: 10px;
      border-top: 1px solid var(--divider-color);
    }
    milo-build-step-entry {
      margin-bottom: 2px;
    }

    .list-entry {
      margin-top: 5px;
    }

    #load {
      color: var(--active-text-color);
    }
  `;
}

customElement('milo-steps-tab')(
  consumeBuildState(
    consumeUserConfigs(
      consumeAppState(StepsTabElement),
    ),
  ),
);
