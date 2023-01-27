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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';

import '../../../components/hotkey';
import './step_display_config';
import './step_list';
import { MiloBaseElement } from '../../../components/milo_base';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../../libs/analytics_utils';
import { consumer } from '../../../libs/context';
import { errorHandler, forwardWithoutMsg, reportRenderError } from '../../../libs/error_handler';
import { consumeStore, StoreInstance } from '../../../store';
import commonStyle from '../../../styles/common_style.css';
import { BuildPageStepListElement } from './step_list';

@customElement('milo-steps-tab')
@errorHandler(forwardWithoutMsg)
@consumer
export class StepsTabElement extends MiloBaseElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  connectedCallback() {
    super.connectedCallback();
    this.store.setSelectedTabId('steps');
    trackEvent(GA_CATEGORIES.STEPS_TAB, GA_ACTIONS.TAB_VISITED, window.location.href);
  }

  private allStepsWereExpanded = false;
  private toggleAllSteps(expand: boolean) {
    this.allStepsWereExpanded = expand;
    this.shadowRoot!.querySelector<BuildPageStepListElement>('milo-bp-step-list')!.toggleAllSteps(expand);
  }
  private readonly toggleAllStepsByHotkey = () => this.toggleAllSteps(!this.allStepsWereExpanded);

  constructor() {
    super();
    makeObservable(this);
  }

  protected render = reportRenderError(this, () => {
    return html`
      <div id="header">
        <milo-bp-step-display-config></milo-bp-step-display-config>
        <milo-hotkey .key=${'x'} .handler=${this.toggleAllStepsByHotkey} title="press x to expand/collapse all entries">
          <mwc-button class="action-button" dense unelevated @click=${() => this.toggleAllSteps(true)}>
            Expand All
          </mwc-button>
          <mwc-button class="action-button" dense unelevated @click=${() => this.toggleAllSteps(false)}>
            Collapse All
          </mwc-button>
        </milo-hotkey>
      </div>
      <milo-bp-step-list id="main" tabindex="0"></milo-bp-step-list>
    `;
  });

  static styles = [
    commonStyle,
    css`
      #header {
        display: grid;
        grid-template-columns: 1fr auto;
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
        width: var(--expand-button-width);
      }

      milo-bp-step-list {
        padding-top: 5px;
        padding-left: 10px;
        outline: none;
      }
    `,
  ];
}
