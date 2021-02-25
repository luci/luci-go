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
import { computed, observable, reaction } from 'mobx';

import '../../components/build_step_entry';
import { BuildStepEntryElement } from '../../components/build_step_entry';
import '../../components/dot_spinner';
import '../../components/hotkey';
import '../../components/lazy_list';
import { AppState, consumeAppState } from '../../context/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state';
import { consumeConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import { BuildStatus } from '../../services/buildbucket';

export class StepsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref buildState!: BuildState;

  private disposer = () => {};
  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'steps';
    trackEvent(GA_CATEGORIES.STEPS_TAB, GA_ACTIONS.TAB_VISITED, window.location.href);

    // Update filters to match the querystring without saving them.
    const searchParams = new URLSearchParams(window.location.search);
    if (searchParams.has('succeeded')) {
      this.stepsConfig.showSucceededSteps = searchParams.get('succeeded') === 'true';
    }
    if (searchParams.has('debug')) {
      this.stepsConfig.showDebugLogs = searchParams.get('debug') === 'true';
    }

    // Update the querystring when filters are updated.
    this.disposer = reaction(
      () => {
        const newSearchParams = new URLSearchParams({
          succeeded: String(this.stepsConfig.showSucceededSteps),
          debug: String(this.stepsConfig.showDebugLogs),
        });
        return newSearchParams.toString();
      },
      (newQueryStr) => {
        const newUrl = `${window.location.protocol}//${window.location.host}${window.location.pathname}?${newQueryStr}`;
        window.history.replaceState({path: newUrl}, '', newUrl);
      },
      {fireImmediately: true},
    );
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    this.disposer();
  }

  @computed private get stepsConfig() {
    return this.configsStore.userConfigs.steps;
  }

  @computed private get loaded() {
    return this.buildState.build !== null;
  }

  @computed private get noDisplayedStep() {
    if (this.stepsConfig.showSucceededSteps) {
      return !this.buildState.build?.rootSteps?.length;
    }
    return !this.buildState.build?.rootSteps?.find((s) => s.status !== BuildStatus.Success);
  }

  private allStepsWereExpanded = false;
  private toggleAllSteps(expand: boolean) {
    this.allStepsWereExpanded = expand;
    this.shadowRoot!.querySelectorAll<BuildStepEntryElement>('milo-build-step-entry')
      .forEach((e) => e.toggleAllSteps(expand));
  }
  private readonly toggleAllStepsByHotkey = () => this.toggleAllSteps(!this.allStepsWereExpanded);

  protected render() {
    return html`
      <div id="header">
        <div class="filters-container">
          Steps:
          <div class="filter">
            <input
              id="succeeded"
              type="checkbox"
              ?checked=${this.stepsConfig.showSucceededSteps}
              @change=${(e: MouseEvent) => {
                this.stepsConfig.showSucceededSteps = (e.target as HTMLInputElement).checked;
                this.configsStore.save();
              }}
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
              ?checked=${this.stepsConfig.showDebugLogs}
              @change=${(e: MouseEvent) => {
                this.stepsConfig.showDebugLogs = (e.target as HTMLInputElement).checked;
                this.configsStore.save();
              }}
            >
            <label for="debug-logs-filter">Debug</label>
          </div>
        </div>
        <span></span>
        <milo-hotkey key="x" .handler=${this.toggleAllStepsByHotkey} title="press x to expand/collapse all entries">
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
      <milo-hotkey
        key="space,shift+space,up,down,pageup,pagedown"
        style="display: none;"
        .handler=${() => this.shadowRoot!.getElementById('main')!.focus()}
      ></milo-hotkey>
      <milo-lazy-list id="main" .growth=${300} tabindex="-1">
        ${this.buildState.build?.rootSteps.map((step, i) => html`
        <milo-build-step-entry
          style=${styleMap({'display': !step.succeededRecursively || this.stepsConfig.showSucceededSteps ? '' : 'none'})}
          .expanded=${!step.succeededRecursively}
          .number=${i + 1}
          .step=${step}
          .prerender=${true}
        ></milo-build-step-entry>
        `) || ''}
        <div
          class="list-entry"
          style=${styleMap({'display': this.loaded && this.noDisplayedStep ? '' : 'none'})}
        >
          ${this.stepsConfig.showSucceededSteps ? 'No steps.' : 'All steps succeeded.'}
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
      grid-template-columns: auto auto auto 1fr auto;
      grid-gap: 5px;
      height: 30px;
      padding: 5px 10px 3px 10px;
    }

    input[type="checkbox"] {
      transform: translateY(1px);
      margin-right: 3px;
    }

    mwc-button {
      margin-top: 1px;
    }

    .filters-container {
      display: inline-block;
      padding: 4px 5px 0;
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
      outline: none;
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
    consumeConfigsStore(
      consumeAppState(StepsTabElement),
    ),
  ),
);
