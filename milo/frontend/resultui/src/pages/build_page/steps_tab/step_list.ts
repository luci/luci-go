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
import { computed, makeObservable, observable, reaction } from 'mobx';

import '../../../components/dot_spinner';
import './step_entry';
import { MiloBaseElement } from '../../../components/milo_base';
import { consumer } from '../../../libs/context';
import { errorHandler, forwardWithoutMsg, reportRenderError } from '../../../libs/error_handler';
import { BuildStatus } from '../../../services/buildbucket';
import { consumeStore, StoreInstance } from '../../../store';
import commonStyle from '../../../styles/common_style.css';
import { BuildPageStepEntryElement } from './step_entry';

@customElement('milo-bp-step-list')
@errorHandler(forwardWithoutMsg)
@consumer
export class BuildPageStepListElement extends MiloBaseElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @computed private get stepsConfig() {
    return this.store.userConfig.build.steps;
  }

  @computed private get loaded() {
    return this.store.buildPage.build !== null;
  }

  @computed private get noStepText() {
    if (!this.loaded) {
      return '';
    }
    const rootSteps = this.store.buildPage.build?.rootSteps;
    if (this.stepsConfig.showSucceededSteps) {
      return !rootSteps?.length ? 'No steps.' : '';
    }
    return !rootSteps?.find((s) => s.status !== BuildStatus.Success) ? 'All steps succeeded.' : '';
  }

  constructor() {
    super();
    makeObservable(this);
  }

  toggleAllSteps(expand: boolean) {
    this.shadowRoot!.querySelectorAll<BuildPageStepEntryElement>('milo-bp-step-entry').forEach((e) =>
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
      ${this.store.buildPage.build?.rootSteps.map(
        (step) => html`<milo-bp-step-entry .step=${step}></milo-bp-step-entry>`
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

      milo-bp-step-entry {
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
