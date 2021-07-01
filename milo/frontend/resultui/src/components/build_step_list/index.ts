// Copyright 2021 The LUCI Authors.
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
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction } from 'mobx';

import './build_step_entry';
import '../dot_spinner';
import { BuildState, consumeBuildState } from '../../context/build_state';
import { consumeConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { consumer } from '../../libs/context';
import { errorHandler, forwardWithoutMsg, reportRenderError } from '../../libs/error_handler';
import { BuildStatus } from '../../services/buildbucket';
import commonStyle from '../../styles/common_style.css';
import { MiloBaseElement } from '../milo_base';
import { BuildStepEntryElement } from './build_step_entry';

@customElement('milo-build-step-list')
@errorHandler(forwardWithoutMsg)
@consumer
export class BuildStepListElement extends MiloBaseElement {
  @observable.ref
  @consumeConfigsStore()
  configsStore!: UserConfigsStore;

  @observable.ref
  @consumeBuildState()
  buildState!: BuildState;

  @computed private get stepsConfig() {
    return this.configsStore.userConfigs.steps;
  }

  @computed private get loaded() {
    return this.buildState.build !== null;
  }

  @computed private get noStepText() {
    if (!this.loaded) {
      return '';
    }
    const rootSteps = this.buildState.build?.rootSteps;
    if (this.stepsConfig.showSucceededSteps) {
      return !rootSteps?.length ? 'No steps.' : '';
    }
    return !rootSteps?.find((s) => s.status !== BuildStatus.Success) ? 'All steps succeeded.' : '';
  }

  toggleAllSteps(expand: boolean) {
    this.shadowRoot!.querySelectorAll<BuildStepEntryElement>('milo-build-step-entry').forEach((e) =>
      e.toggleAllSteps(expand)
    );
  }

  connectedCallback() {
    super.connectedCallback();
    this.addDisposer(
      reaction(
        () => this.stepsConfig.showSucceededSteps,
        (showSucceededSteps) =>
          this.style.setProperty('--non-critical-build-step-display', showSucceededSteps ? 'block' : 'none'),
        { fireImmediately: true }
      )
    );
  }

  protected render = reportRenderError(this, () => {
    return html`
      ${this.buildState.build?.rootSteps.map(
        (step) => html`<milo-build-step-entry .step=${step}></milo-build-step-entry>`
      ) || ''}
      <div class="list-entry" style=${styleMap({ display: this.noStepText ? '' : 'none' })}>${this.noStepText}</div>
      <div id="load" class="list-entry" style=${styleMap({ display: this.loaded ? 'none' : '' })}>
        Loading <milo-dot-spinner></milo-dot-spinner>
      </div>
    `;
  });

  static styles = [
    commonStyle,
    css`
      :host {
        display: block;
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
    `,
  ];
}
