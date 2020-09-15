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
import '@material/mwc-icon';
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';

import '../../components/build_step_entry';
import { BuildStepEntryElement } from '../../components/build_step_entry';
import '../../components/dot_spinner';
import { AppState, consumeAppState } from '../../context/app_state/app_state';
import { BuildState, consumeBuildState } from '../../context/build_state/build_state';
import { BuildStatus } from '../../services/buildbucket';

export class StepsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref buildState!: BuildState;

  // TODO(crbug/1123362): save the setting.
  @observable.ref showSucceeded = true;
  @observable.ref showDebugLogs = false;
  @observable.ref expandNonLeaf = false;

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'steps';
  }

  @computed private get loaded() {
    return this.buildState.buildPageData !== null;
  }

  @computed private get noDisplayedStep() {
    if (this.showSucceeded) {
      return !this.buildState.buildPageData?.steps?.length;
    }
    return !this.buildState.buildPageData?.steps?.find((s) => s.status !== BuildStatus.Success);
  }

  private toggleAllSteps(expand: boolean) {
    this.shadowRoot!.querySelectorAll<BuildStepEntryElement>('milo-build-step-entry')
      .forEach((e) => e.toggleAllSteps(expand));
  }

  protected render() {
    // TODO(crbug/1123362): add expand/collapse all buttons.
    return html`
      <div id="header">
        <div class="toggles-container">
          Steps:
          <div class="toggle">
            <input
              id="succeeded"
              type="checkbox"
              ?checked=${this.showSucceeded}
              @change=${(e: MouseEvent) => this.showSucceeded = (e.target as HTMLInputElement).checked}
            >
            <label for="succeeded" style="color: var(--success-color);">Succeeded</label>
          </div>
          <div class="toggle">
            <input id="others" type="checkbox" disabled checked>
            <label for="others">Others</label>
          </div>
        </div>
        <div class="toggles-container-delimiter"></div>
        <div class="toggles-container">
          <div class="toggle">
            <input
              id="expand-non-leaf"
              type="checkbox"
              ?checked=${this.expandNonLeaf}
              @change=${(e: MouseEvent) => this.expandNonLeaf = (e.target as HTMLInputElement).checked}
            >
            <label for="expand-non-leaf">
              Expand Non-Leaf
              <mwc-icon id="expand-non-leaf-info" title="Expand steps with sub-steps by default.">info</mwc-icon>
            </label>
          </div>
          <div class="toggle">
            <input
              id="debug-logs-filter"
              type="checkbox"
              ?checked=${this.showDebugLogs}
              @change=${(e: MouseEvent) => this.showDebugLogs = (e.target as HTMLInputElement).checked}
            >
            <label for="debug-logs-filter">Debug Logs</label>
          </div>
        </div>
        <span></span>
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
      </div>
      <div id="main">
        ${this.buildState.buildPageData?.steps?.map((step, i) => html`
        <milo-build-step-entry
          style=${styleMap({'display': step.status !== BuildStatus.Success || this.showSucceeded ? '' : 'none'})}
          .expanded=${(this.expandNonLeaf || !step.children?.length)}
          .number=${i + 1}
          .step=${step}
          .showDebugLogs=${this.showDebugLogs}
        ></milo-build-step-entry>
        `) || ''}
        <div
          class="list-entry"
          style=${styleMap({'display': this.loaded && this.noDisplayedStep ? '' : 'none'})}
        >
          ${this.showSucceeded ? 'No steps.' : 'All steps succeeded.'}
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
      grid-template-columns: auto auto auto 1fr auto auto;
      grid-gap: 5px;
      height: 28px;
      padding: 5px 10px 3px 10px;
    }

    .toggles-container {
      display: inline-block;
      padding: 0 5px;
      padding-top: 5px;
    }
    .toggle {
      display: inline-block;
      margin: 0 5px;
    }
    .toggle:last-child {
      margin-right: 0px;
    }
    .toggles-container-delimiter {
      border-left: 1px solid var(--divider-color);
      width: 0px;
      height: 100%;
    }

    #expand-non-leaf-info {
      color: #212121;
      --mdc-icon-size: 1.2em;
      vertical-align: bottom;
    }

    .action-button {
      --mdc-theme-primary: rgb(0, 123, 255);
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
    consumeAppState(StepsTabElement),
  ),
);
