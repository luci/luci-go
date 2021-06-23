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

import '@material/mwc-icon';
import { css, customElement } from 'lit-element';
import { html, render } from 'lit-html';
import { classMap } from 'lit-html/directives/class-map';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction } from 'mobx';

import '../copy_to_clipboard';
import '../expandable_entry';
import '../log';
import '../pin_toggle';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state';
import { consumeConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP, BUILD_STATUS_ICON_MAP } from '../../libs/constants';
import { lazyRendering, RenderPlaceHolder } from '../../libs/observer_element';
import { displayCompactDuration, displayDuration, NUMERIC_TIME_FORMAT } from '../../libs/time_utils';
import { StepExt } from '../../models/step_ext';
import { BuildStatus } from '../../services/buildbucket';
import colorClasses from '../../styles/color_classes.css';
import commonStyle from '../../styles/common_style.css';
import { MiloBaseElement } from '../milo_base';
import { HideTooltipEventDetail, ShowTooltipEventDetail } from '../tooltip';

/**
 * Renders a step.
 */
@customElement('milo-build-step-entry')
@lazyRendering
export class BuildStepEntryElement extends MiloBaseElement implements RenderPlaceHolder {
  @observable.ref
  @consumeConfigsStore()
  configsStore!: UserConfigsStore;

  @observable.ref
  @consumeInvocationState()
  invState!: InvocationState;

  @observable.ref step!: StepExt;

  @observable.ref private _expanded = false;
  get expanded() {
    return this._expanded;
  }
  set expanded(newVal) {
    this._expanded = newVal;
    // Always render the content once it was expanded so the descendants' states
    // don't get reset after the node is collapsed.
    this.shouldRenderContent = this.shouldRenderContent || newVal;
  }

  @observable.ref private shouldRenderContent = false;

  @computed private get isPinned() {
    return this.configsStore.stepIsPinned(this.step.name);
  }

  @computed private get isCriticalStep() {
    return this.step.status !== BuildStatus.Success || this.isPinned;
  }

  private expandSubSteps = false;
  toggleAllSteps(expand: boolean) {
    this.expanded = expand;
    this.expandSubSteps = expand;
    this.shadowRoot!.querySelectorAll<BuildStepEntryElement>('milo-build-step-entry').forEach((e) =>
      e.toggleAllSteps(expand)
    );
  }

  @computed private get logs() {
    const logs = this.step.logs || [];
    return this.configsStore.userConfigs.steps.showDebugLogs ? logs : logs.filter((log) => !log.name.startsWith('$'));
  }

  private renderContent() {
    if (!this.shouldRenderContent) {
      return html``;
    }
    return html`
      <div
        id="summary"
        class="${BUILD_STATUS_CLASS_MAP[this.step.status]}-bg"
        style=${styleMap({ display: this.step.summary ? '' : 'none' })}
      >
        ${this.step.summary}
      </div>
      <ul id="log-links" style=${styleMap({ display: this.step.logs?.length ? '' : 'none' })}>
        ${this.logs.map((log) => html`<li><milo-log .log=${log}></li>`)}
      </ul>
      ${this.step.children?.map(
        (child) => html`
          <milo-build-step-entry .step=${child} .expanded=${this.expandSubSteps}></milo-build-step-entry>
        `
      ) || ''}
    `;
  }

  private renderDuration() {
    if (!this.step.startTime) {
      return html` <span class="badge" title="No duration">N/A</span> `;
    }

    return html`
      <div
        class="badge"
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
        ${displayCompactDuration(this.step.duration)}
      </div>
    `;
  }

  private renderDurationTooltip() {
    if (!this.step.startTime) {
      return html``;
    }
    return html`
      <table>
        <tr>
          <td>Started:</td>
          <td>${this.step.startTime.toFormat(NUMERIC_TIME_FORMAT)}</td>
        </tr>
        <tr>
          <td>Ended:</td>
          <td>${this.step.endTime ? this.step.endTime.toFormat(NUMERIC_TIME_FORMAT) : 'N/A'}</td>
        </tr>
        <tr>
          <td>Duration:</td>
          <td>${displayDuration(this.step.duration)}</td>
        </tr>
      </div>
    `;
  }

  private trackInteraction = () => {
    if (this.invState.testLoader?.hasAssociatedUnexpectedResults(this.step.name)) {
      trackEvent(GA_CATEGORIES.STEP_WITH_UNEXPECTED_RESULTS, GA_ACTIONS.INSPECT_TEST, VISIT_ID);
    }
  };

  connectedCallback() {
    super.connectedCallback();
    this.addDisposer(
      reaction(
        () => this.isCriticalStep,
        () => {
          this.style['display'] = `var(--${this.isCriticalStep ? '' : 'non-'}critical-build-step-display, block)`;
        },
        { fireImmediately: true }
      )
    );
    this.addDisposer(
      reaction(
        () => this.configsStore.userConfigs.steps.expandByDefault,
        (expandByDefault) => {
          this.expanded = expandByDefault;
          this.expandSubSteps = true;
        },
        { fireImmediately: true }
      )
    );

    this.addEventListener('click', this.trackInteraction);
    this.addDisposer(() => this.removeEventListener('click', this.trackInteraction));
  }

  firstUpdated() {
    if (!this.step.succeededRecursively) {
      this.expanded = true;
    }
    if (this.isPinned) {
      this.expanded = true;

      // Keep the pin setting fresh.
      this.configsStore.setStepPin(this.step.name, this.isPinned);
    }
  }

  renderPlaceHolder() {
    return '';
  }

  protected render() {
    return html`
      <milo-expandable-entry .expanded=${this.expanded} .onToggle=${(expanded: boolean) => (this.expanded = expanded)}>
        <span id="header" slot="header">
          <mwc-icon
            id="status-indicator"
            class=${BUILD_STATUS_CLASS_MAP[this.step.status]}
            title=${BUILD_STATUS_DISPLAY_MAP[this.step.status]}
          >
            ${BUILD_STATUS_ICON_MAP[this.step.status]}
          </mwc-icon>
          ${this.renderDuration()}
          <div
            id="header-text"
            class=${classMap({
              [`${BUILD_STATUS_CLASS_MAP[this.step.status]}-bg`]:
                this.step.status !== BuildStatus.Success && !(this.expanded && this.step.summary),
            })}
          >
            <b>${this.step.index + 1}. ${this.step.selfName}</b>
            <milo-pin-toggle
              .pinned=${this.isPinned}
              title="Pin/unpin the step. The configuration is shared across all builds."
              class="hidden-icon"
              style=${styleMap({ visibility: this.isPinned ? 'visible' : '' })}
              @click=${(e: Event) => {
                this.configsStore.setStepPin(this.step.name, !this.isPinned);
                e.stopPropagation();
                // Users are not consuming the step info when (un)setting the
                // pins, don't record step interaction.
              }}
            >
            </milo-pin-toggle>
            <milo-copy-to-clipboard
              .textToCopy=${this.step.name}
              title="Copy the step name."
              class="hidden-icon"
              @click=${(e: Event) => {
                e.stopPropagation();
                this.trackInteraction();
              }}
            ></milo-copy-to-clipboard>
            <span id="header-markdown">${this.step.header}</span>
          </div>
        </span>
        <div id="content" slot="content">${this.renderContent()}</div>
      </milo-expandable-entry>
    `;
  }

  static styles = [
    commonStyle,
    colorClasses,
    css`
      :host {
        display: none;
        min-height: 24px;
      }

      #header {
        display: inline-grid;
        grid-template-columns: auto auto 1fr;
        grid-gap: 5px;
        width: 100%;
        overflow: hidden;
        text-overflow: ellipsis;
      }
      .hidden-icon {
        visibility: hidden;
      }
      #header:hover .hidden-icon {
        visibility: visible;
      }
      #header.success > b {
        color: var(--default-text-color);
      }

      #status-indicator {
        vertical-align: bottom;
      }

      .badge {
        margin-top: 3px;
        margin-bottom: 5px;
      }

      #header-text {
        padding-left: 4px;
        box-sizing: border-box;
        height: 24px;
        overflow: hidden;
        text-overflow: ellipsis;
      }

      #header-markdown * {
        display: inline;
      }

      #content {
        margin-top: 2px;
        overflow: hidden;
      }

      #summary {
        padding: 5px;
        clear: both;
        overflow-wrap: break-word;
      }

      #summary > p:first-child {
        margin-block-start: 0px;
      }

      #summary > :last-child {
        margin-block-end: 0px;
      }

      #summary a {
        color: var(--default-text-color);
      }

      #log-links {
        margin: 3px 0;
        padding-inline-start: 28px;
        clear: both;
        overflow-wrap: break-word;
      }

      #log-links > li {
        list-style-type: circle;
      }

      milo-build-step-entry {
        margin-bottom: 2px;

        /* Always render all child steps. */
        --non-critical-build-step-display: block;
        --critical-build-step-display: block;
      }
    `,
  ];
}
