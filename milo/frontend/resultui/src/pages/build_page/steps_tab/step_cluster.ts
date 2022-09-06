// Copyright 2022 The LUCI Authors.
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

import '@material/mwc-icon';
import { css, customElement, html } from 'lit-element';
import { render } from 'lit-html';
import { styleMap } from 'lit-html/directives/style-map';
import { Duration } from 'luxon';
import { action, computed, makeObservable, observable, reaction } from 'mobx';

import './step_entry';
import { MiloBaseElement } from '../../../components/milo_base';
import { HideTooltipEventDetail, ShowTooltipEventDetail } from '../../../components/tooltip';
import { consumer } from '../../../libs/context';
import { displayCompactDuration, displayDuration } from '../../../libs/time_utils';
import { consumeStore, StoreInstance } from '../../../store';
import { StepExt } from '../../../store/build_state';
import commonStyle from '../../../styles/common_style.css';
import { BuildPageStepEntryElement } from './step_entry';

@customElement('milo-bp-step-cluster')
@consumer
export class BuildPageStepClusterElement extends MiloBaseElement {
  @observable.ref @consumeStore() store!: StoreInstance;
  @observable.ref steps!: readonly StepExt[];

  @observable.ref private expanded = false;

  @computed private get shouldElide() {
    return (
      this.steps.length > 1 &&
      !this.steps[0].isCritical &&
      this.store.userConfig.build.steps.elideSucceededSteps &&
      !this.expanded
    );
  }

  @computed private get stepWithDurationCount() {
    return this.steps.filter((s) => s.startTime).length;
  }

  @computed private get totalDuration() {
    return this.steps.reduce((total, s) => total.plus(s.duration), Duration.fromMillis(0));
  }

  @computed private get avgDuration() {
    return Duration.fromMillis(this.totalDuration.toMillis() / this.stepWithDurationCount);
  }

  @action private setExpanded(expand: boolean) {
    this.expanded = expand;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  private expandSteps = false;
  toggleAllSteps(expand: boolean) {
    this.expandSteps = expand;
    this.setExpanded(expand);
    this.shadowRoot!.querySelectorAll<BuildPageStepEntryElement>('milo-bp-step-entry').forEach((e) =>
      e.toggleAllSteps(expand)
    );
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.store.userConfig.build.steps.elideSucceededSteps,
        (elideSucceededSteps) => this.setExpanded(!elideSucceededSteps)
      )
    );
  }

  protected render() {
    return html`${this.renderElidedSteps()}${this.renderSteps()}`;
  }

  private renderElidedSteps() {
    if (!this.shouldElide) {
      return;
    }

    const firstStepLabel = this.steps[0].index + 1;
    const lastStepLabel = this.steps[this.steps.length - 1].index + 1;

    return html`
      <div id="elided-steps" @click=${() => this.setExpanded(true)}>
        <mwc-icon>more_horiz</mwc-icon>
        <svg width="24" height="24">
          <image xlink:href="/ui/immutable/svgs/check_circle_stacked_24dp.svg" width="24" height="24" />
        </svg>
        ${this.renderDuration()}
        <div id="elided-steps-description">Step ${firstStepLabel} ~ ${lastStepLabel} succeeded.</div>
      </div>
    `;
  }

  private renderDuration() {
    const [compactDuration, compactDurationUnits] = displayCompactDuration(this.totalDuration);

    return html`
      <div
        class="duration ${compactDurationUnits}"
        @mouseover=${(e: MouseEvent) => {
          const tooltip = document.createElement('div');
          render(this.renderDurationTooltip(), tooltip);

          window.dispatchEvent(
            new CustomEvent<ShowTooltipEventDetail>('show-tooltip', {
              detail: {
                tooltip,
                targetRect: (e.target as HTMLElement).getBoundingClientRect(),
                gapSize: 5,
              },
            })
          );
        }}
        @mouseout=${() => {
          window.dispatchEvent(new CustomEvent<HideTooltipEventDetail>('hide-tooltip', { detail: { delay: 50 } }));
        }}
      >
        ${compactDuration}
      </div>
    `;
  }

  private renderDurationTooltip() {
    return html`
      <table>
        <tr>
          <td>Total Duration:</td>
          <td>${displayDuration(this.totalDuration)}</td>
        </tr>
        <tr>
          <td>Average Duration:</td>
          <td>${displayDuration(this.avgDuration)}</td>
        </tr>
        <tr>
          <td>Steps w/o Duration:</td>
          <td>${this.steps.length - this.stepWithDurationCount}</td>
        </tr>
      </div>
    `;
  }

  private renderedSteps = false;
  private renderSteps() {
    if (!this.renderedSteps && this.shouldElide) {
      return;
    }
    // Once rendered to DOM, always render to DOM since we have done the hard
    // work.
    this.renderedSteps = true;

    return html`
      <div style=${styleMap({ display: this.shouldElide ? 'none' : 'block' })}>
        ${this.steps.map(
          (step) => html`<milo-bp-step-entry .step=${step} .expanded=${this.expandSteps}></milo-bp-step-entry>`
        )}
      </div>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: block;
      }

      #elided-steps {
        display: grid;
        grid-template-columns: auto auto auto 1fr;
        grid-gap: 5px;
        height: 24px;
        line-height: 24px;
        cursor: pointer;
      }

      .duration {
        margin-top: 3px;
        margin-bottom: 5px;
      }

      #elided-steps-description {
        padding-left: 4px;
        font-weight: bold;
        font-style: italic;
      }

      milo-bp-step-entry {
        margin-bottom: 2px;
      }
    `,
  ];
}
