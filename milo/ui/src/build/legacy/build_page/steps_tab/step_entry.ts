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
import { css, html, render } from 'lit';
import { customElement } from 'lit/decorators.js';
import { styleMap } from 'lit/directives/style-map.js';
import { computed, makeObservable, observable, reaction } from 'mobx';

import '@/generic_libs/components/copy_to_clipboard';
import '@/generic_libs/components/expandable_entry';
import '@/common/components/buildbucket_log_link';
import '@/common/components/instruction_hint';
import '@/generic_libs/components/pin_toggle';
import './step_cluster';

import {
  HideTooltipEventDetail,
  ShowTooltipEventDetail,
} from '@/common/components/tooltip';
import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_DISPLAY_MAP,
  BUILD_STATUS_ICON_MAP,
} from '@/common/constants/legacy';
import { BuildbucketStatus } from '@/common/services/buildbucket';
import { consumeStore, StoreInstance } from '@/common/store';
import { StepExt } from '@/common/store/build_state';
import { ExpandStepOption } from '@/common/store/user_config/build_config';
import { colorClasses, commonStyles } from '@/common/styles/stylesheets';
import {
  displayCompactDuration,
  displayDuration,
  NUMERIC_TIME_FORMAT,
} from '@/common/tools/time_utils';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import {
  lazyRendering,
  RenderPlaceHolder,
} from '@/generic_libs/tools/observer_element';

import { BuildPageStepClusterElement } from './step_cluster';

/**
 * Renders a step.
 */
@customElement('milo-bp-step-entry')
@lazyRendering
export class BuildPageStepEntryElement
  extends MobxExtLitElement
  implements RenderPlaceHolder
{
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref step!: StepExt;

  @observable.ref private _expanded = false;

  @computed get expanded() {
    return this._expanded;
  }
  set expanded(newVal) {
    this._expanded = newVal;
    // Always render the content once it was expanded so the descendants' states
    // don't get reset after the node is collapsed.
    this.shouldRenderContent = this.shouldRenderContent || newVal;
  }

  @observable.ref private shouldRenderContent = false;

  toggleAllSteps(expand: boolean) {
    this.expanded = expand;
    this.shadowRoot!.querySelectorAll<BuildPageStepClusterElement>(
      'milo-bp-step-cluster',
    ).forEach((e) => e.toggleAllSteps(expand));
  }

  constructor() {
    super();
    makeObservable(this);
  }

  private renderContent() {
    if (!this.shouldRenderContent) {
      return html``;
    }
    // We have to cloneNode below because otherwise lit 'uses' the HTML elements
    // and if we try to render them a second time we get an empty box.
    return html`
      <div
        id="summary"
        style=${styleMap({
          display: this.step.summary ? '' : 'none',
          backgroundColor: 'var(--block-background-color)',
        })}
      >
        ${this.step.summary?.cloneNode(true)}
      </div>
      <ul
        id="log-links"
        style=${styleMap({
          display: this.step.filteredLogs.length ? '' : 'none',
        })}
      >
        ${this.step.filteredLogs.map(
          (log) =>
            html`<li>
              <milo-buildbucket-log-link
                .log=${log}
              ></milo-buildbucket-log-link>
            </li>`,
        )}
      </ul>
      ${this.step.tags.length
        ? html`<milo-tags-entry .tags=${this.step.tags}></milo-tags-entry>`
        : ''}
      ${this.step.clusteredChildren.map(
        (cluster) =>
          html`<milo-bp-step-cluster .steps=${cluster}></milo-bp-step-cluster>`,
      ) || ''}
    `;
  }

  private renderDuration() {
    if (!this.step.startTime) {
      return html` <span class="duration" title="No duration">N/A</span> `;
    }

    const [compactDuration, compactDurationUnits] = displayCompactDuration(
      this.step.duration,
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
          <td>${
            this.step.endTime
              ? this.step.endTime.toFormat(NUMERIC_TIME_FORMAT)
              : 'N/A'
          }</td>
        </tr>
        <tr>
          <td>Duration:</td>
          <td>${displayDuration(this.step.duration)}</td>
        </tr>
      </div>
    `;
  }

  connectedCallback() {
    super.connectedCallback();
    this.addDisposer(
      reaction(
        () => this.store.userConfig.build.steps.expandByDefault,
        (opt) => {
          switch (opt) {
            case ExpandStepOption.All:
              this.expanded = true;
              break;
            case ExpandStepOption.None:
              this.expanded = false;
              break;
            case ExpandStepOption.NonSuccessful:
              this.expanded = this.step.status !== BuildbucketStatus.Success;
              break;
            case ExpandStepOption.WithNonSuccessful:
              this.expanded = !this.step.succeededRecursively;
              break;
          }
        },
        { fireImmediately: true },
      ),
    );
  }

  firstUpdated() {
    if (this.step.isPinned) {
      this.expanded = true;

      // Keep the pin setting fresh.
      this.step.setIsPinned(this.step.isPinned);
    }
  }

  renderPlaceHolder() {
    return '';
  }

  protected render() {
    return html`
      <milo-expandable-entry
        .expanded=${this.expanded}
        .onToggle=${(expanded: boolean) => (this.expanded = expanded)}
      >
        <span id="header" slot="header">
          <mwc-icon
            id="status-indicator"
            class=${BUILD_STATUS_CLASS_MAP[this.step.status]}
            title=${BUILD_STATUS_DISPLAY_MAP[this.step.status]}
          >
            ${BUILD_STATUS_ICON_MAP[this.step.status]}
          </mwc-icon>
          ${this.renderDuration()}
          <div id="header-text">
            <b>${this.step.index + 1}. ${this.step.selfName}</b>
            ${this.step.instructionID
              ? html`<milo-instruction-hint
                  instruction-name=${'invocations/build-' +
                  this.store.buildPage.build!.data.id +
                  '/instructions/' +
                  this.step.instructionID}
                  title="How to reproduce this step"
                ></milo-instruction-hint>`
              : html``}
            <milo-pin-toggle
              .pinned=${this.step.isPinned}
              title="Pin/unpin the step. The configuration is shared across all builds."
              class="hidden-icon"
              style=${styleMap({
                visibility: this.step.isPinned ? 'visible' : '',
              })}
              @click=${(e: Event) => {
                this.step.setIsPinned(!this.step.isPinned);
                e.stopPropagation();
              }}
            >
            </milo-pin-toggle>
            <milo-copy-to-clipboard
              .textToCopy=${this.step.name}
              title="Copy the step name."
              class="hidden-icon"
              @click=${(e: Event) => e.stopPropagation()}
            ></milo-copy-to-clipboard>
            <span id="header-markdown"
              >${this.expanded ? null : this.step.summary}</span
            >
            ${!this.expanded && this.step.summary?.title
              ? html` <milo-copy-to-clipboard
                  .textToCopy=${this.step.summary.title}
                  title="Copy the step summary."
                  class="hidden-icon"
                  @click=${(e: Event) => e.stopPropagation()}
                ></milo-copy-to-clipboard>`
              : html``}
          </div>
        </span>
        <div id="content" slot="content">${this.renderContent()}</div>
      </milo-expandable-entry>
    `;
  }

  static styles = [
    commonStyles,
    colorClasses,
    css`
      :host {
        display: block;
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

      .duration {
        margin-top: 3px;
        margin-bottom: 5px;
      }

      #header-text {
        padding-left: 4px;
        box-sizing: border-box;
        height: 24px;
        overflow: hidden;
        text-overflow: ellipsis;
        display: grid;
        grid-template-columns: repeat(6, auto) 1fr;
      }

      #header-markdown {
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
    `,
  ];
}
