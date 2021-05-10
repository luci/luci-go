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

import '@material/mwc-button';
import { css, customElement, html } from 'lit-element';
import { computed, observable } from 'mobx';

import '../../components/build_step_list';
import { BuildStepEntryElement } from '../../components/build_step_list/build_step_entry';
import { MiloBaseElement } from '../../components/milo_base';
import { AppState, consumeAppState } from '../../context/app_state';
import { consumeConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import { consumer } from '../../libs/context';
import { errorHandler, forwardWithoutMsg, reportRenderError } from '../../libs/error_handler';
import commonStyle from '../../styles/common_style.css';

@customElement('milo-steps-tab')
@errorHandler(forwardWithoutMsg)
@consumer
export class StepsTabElement extends MiloBaseElement {
  @observable.ref
  @consumeAppState
  appState!: AppState;

  @observable.ref
  @consumeConfigsStore
  configsStore!: UserConfigsStore;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'steps';
    trackEvent(GA_CATEGORIES.STEPS_TAB, GA_ACTIONS.TAB_VISITED, window.location.href);
  }

  @computed private get stepsConfig() {
    return this.configsStore.userConfigs.steps;
  }

  private allStepsWereExpanded = false;
  private toggleAllSteps(expand: boolean) {
    this.allStepsWereExpanded = expand;
    this.shadowRoot!.querySelector<BuildStepEntryElement>('milo-build-step-list')!.toggleAllSteps(expand);
  }
  private readonly toggleAllStepsByHotkey = () => this.toggleAllSteps(!this.allStepsWereExpanded);

  protected render = reportRenderError.bind(this)(() => {
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
              }}
            />
            <label for="succeeded" style="color: var(--success-color);">Succeeded</label>
          </div>
          <div class="filter">
            <input id="others" type="checkbox" disabled checked />
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
              }}
            />
            <label for="debug-logs-filter">Debug</label>
          </div>
        </div>
        <span></span>
        <milo-hotkey key="x" .handler=${this.toggleAllStepsByHotkey} title="press x to expand/collapse all entries">
          <mwc-button class="action-button" dense unelevated @click=${() => this.toggleAllSteps(true)}>
            Expand All
          </mwc-button>
          <mwc-button class="action-button" dense unelevated @click=${() => this.toggleAllSteps(false)}>
            Collapse All
          </mwc-button>
        </milo-hotkey>
      </div>
      <milo-build-step-list id="main" tabindex="0"></milo-build-step-list>
    `;
  });

  static styles = [
    commonStyle,
    css`
      #header {
        display: grid;
        grid-template-columns: auto auto auto 1fr auto;
        grid-gap: 5px;
        height: 30px;
        padding: 5px 10px 3px 10px;
        position: sticky;
        top: 0px;
        background: white;
        border-bottom: 1px solid var(--divider-color);
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

      milo-build-step-list {
        padding-top: 5px;
        padding-left: 10px;
        outline: none;
      }
    `,
  ];
}
