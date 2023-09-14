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

import '@/generic_libs/components/hotkey';
import './step_display_config';
import './step_list';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { consumeStore, StoreInstance } from '@/common/store';
import { commonStyles } from '@/common/styles/stylesheets';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { useTabId } from '@/generic_libs/components/routed_tabs';
import {
  errorHandler,
  forwardWithoutMsg,
  reportRenderError,
} from '@/generic_libs/tools/error_handler';
import { consumer } from '@/generic_libs/tools/lit_context';

import { BuildPageStepListElement } from './step_list';

@customElement('milo-steps-tab')
@errorHandler(forwardWithoutMsg)
@consumer
export class StepsTabElement extends MobxExtLitElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  private allStepsWereExpanded = false;
  private toggleAllSteps(expand: boolean) {
    this.allStepsWereExpanded = expand;
    this.shadowRoot!.querySelector<BuildPageStepListElement>(
      'milo-bp-step-list',
    )!.toggleAllSteps(expand);
  }
  private readonly toggleAllStepsByHotkey = () =>
    this.toggleAllSteps(!this.allStepsWereExpanded);

  constructor() {
    super();
    makeObservable(this);
  }

  protected render = reportRenderError(this, () => {
    return html`
      <div id="header">
        <milo-bp-step-display-config></milo-bp-step-display-config>
        <milo-hotkey
          .key=${'x'}
          .handler=${this.toggleAllStepsByHotkey}
          title="press x to expand/collapse all entries"
        >
          <mwc-button
            class="action-button"
            dense
            unelevated
            @click=${() => this.toggleAllSteps(true)}
          >
            Expand All
          </mwc-button>
          <mwc-button
            class="action-button"
            dense
            unelevated
            @click=${() => this.toggleAllSteps(false)}
          >
            Collapse All
          </mwc-button>
        </milo-hotkey>
      </div>
      <milo-bp-step-list id="main" tabindex="0"></milo-bp-step-list>
    `;
  });

  static styles = [
    commonStyles,
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

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-steps-tab': Record<string, never>;
    }
  }
}

export function StepsTab() {
  return <milo-steps-tab />;
}

export function Component() {
  useTabId('steps');

  return (
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="steps">
      <StepsTab />
    </RecoverableErrorBoundary>
  );
}
