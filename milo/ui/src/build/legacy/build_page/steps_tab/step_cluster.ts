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
import { css, html, render } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { DateTime } from 'luxon';
import { action, computed, makeObservable, observable, reaction } from 'mobx';

import './step_entry';
import checkCircleStacked from '@/common/assets/svgs/check_circle_stacked_24dp.svg';
import {
  HideTooltipEventDetail,
  ShowTooltipEventDetail,
} from '@/common/components/tooltip';
import { consumeStore, StoreInstance } from '@/common/store';
import { StepExt } from '@/common/store/build_state';
import { commonStyles } from '@/common/styles/stylesheets';
import {
  displayCompactDuration,
  displayDuration,
  NUMERIC_TIME_FORMAT,
} from '@/common/tools/time_utils';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { consumer } from '@/generic_libs/tools/lit_context';

import { BuildPageStepEntryElement } from './step_entry';

@customElement('milo-bp-step-cluster')
@consumer
export class BuildPageStepClusterElement extends MobxExtLitElement {
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

  @computed private get startTime() {
    return this.steps.reduce((earliest: DateTime | null, step) => {
      if (!earliest) {
        return step.startTime;
      }
      if (!step.startTime) {
        return earliest;
      }
      return step.startTime < earliest ? step.startTime : earliest;
    }, null);
  }

  @computed private get endTime() {
    return this.steps.reduce((latest: DateTime | null, step) => {
      if (!latest) {
        return step.endTime;
      }
      if (!step.endTime) {
        return latest;
      }
      return step.endTime > latest ? step.endTime : latest;
    }, null);
  }

  @computed get duration() {
    if (!this.startTime || !this.endTime) {
      return null;
    }

    return this.endTime.diff(this.startTime);
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
    this.shadowRoot!.querySelectorAll<BuildPageStepEntryElement>(
      'milo-bp-step-entry',
    ).forEach((e) => e.toggleAllSteps(expand));
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => this.store.userConfig.build.steps.elideSucceededSteps,
        (elideSucceededSteps) => this.setExpanded(!elideSucceededSteps),
      ),
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
          <image href=${checkCircleStacked} width="24" height="24" />
        </svg>
        ${this.renderDuration()}
        <div id="elided-steps-description">
          Step ${firstStepLabel} ~ ${lastStepLabel} succeeded.
        </div>
      </div>
    `;
  }

  private renderDuration() {
    const [compactDuration, compactDurationUnits] = displayCompactDuration(
      this.duration,
    );

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
            }),
          );
        }}
        @mouseout=${() => {
          window.dispatchEvent(
            new CustomEvent<HideTooltipEventDetail>('hide-tooltip', {
              detail: { delay: 50 },
            }),
          );
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
          <td>Started:</td>
          <td>${
            this.startTime
              ? this.startTime.toFormat(NUMERIC_TIME_FORMAT)
              : 'N/A'
          }</td>
        </tr>
        <tr>
          <td>Ended:</td>
          <td>${
            this.endTime ? this.endTime.toFormat(NUMERIC_TIME_FORMAT) : 'N/A'
          }</td>
        </tr>
        <tr>
          <td>Duration:</td>
          <td>${this.duration ? displayDuration(this.duration) : 'N/A'}</td>
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
          (step) =>
            html`<milo-bp-step-entry
              .step=${step}
              .expanded=${this.expandSteps}
            ></milo-bp-step-entry>`,
        )}
      </div>
    `;
  }

  static styles = [
    commonStyles,
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
