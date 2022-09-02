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
import { computed, makeObservable, observable } from 'mobx';

import '../../../components/dot_spinner';
import './step_cluster';
import { MiloBaseElement } from '../../../components/milo_base';
import { consumer } from '../../../libs/context';
import { errorHandler, forwardWithoutMsg, reportRenderError } from '../../../libs/error_handler';
import { BuildStatus } from '../../../services/buildbucket';
import { consumeStore, StoreInstance } from '../../../store';
import commonStyle from '../../../styles/common_style.css';
import { BuildPageStepClusterElement } from './step_cluster';

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
    return !rootSteps?.find((s) => s.data.status !== BuildStatus.Success) ? 'All steps succeeded.' : '';
  }

  constructor() {
    super();
    makeObservable(this);
  }

  private expandSubSteps = false;
  toggleAllSteps(expand: boolean) {
    this.expandSubSteps = expand;
    this.shadowRoot!.querySelectorAll<BuildPageStepClusterElement>('milo-bp-step-cluster').forEach((e) =>
      e.toggleAllSteps(expand)
    );
  }

  protected render = reportRenderError(this, () => {
    return html`
      ${this.store.buildPage.build?.clusteredRootSteps.map(
        (cluster) =>
          html`<milo-bp-step-cluster .steps=${cluster} .expanded=${this.expandSubSteps}></milo-bp-step-cluster>`
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

      .list-entry {
        margin-top: 5px;
      }

      #load {
        color: var(--active-text-color);
      }
    `,
  ];
}
