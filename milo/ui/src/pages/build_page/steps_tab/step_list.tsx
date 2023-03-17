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
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';

import '../../../components/dot_spinner';
import './step_cluster';
import { MiloBaseElement } from '../../../components/milo_base';
import { consumer } from '../../../libs/context';
import { errorHandler, forwardWithoutMsg, reportRenderError } from '../../../libs/error_handler';
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
    const build = this.store.buildPage.build;
    if (!build) {
      return html` <div id="load" class="list-entry">Loading <milo-dot-spinner></milo-dot-spinner></div> `;
    }

    if (build.rootSteps.length === 0) {
      return html` <div class="list-entry">No steps.</div> `;
    }

    return html`
      ${build.clusteredRootSteps.map(
        (cluster) =>
          html`<milo-bp-step-cluster .steps=${cluster} .expanded=${this.expandSubSteps}></milo-bp-step-cluster>`
      )}
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

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-bp-step-list': {};
    }
  }
}

export function StepList() {
  return <milo-bp-step-list />;
}
