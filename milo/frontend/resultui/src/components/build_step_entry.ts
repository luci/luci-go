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

import { MobxLitElement } from '@adobe/lit-mobx';
import '@material/mwc-icon';
import { css, customElement } from 'lit-element';
import { html } from 'lit-html';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';

import '../components/copy_to_clipboard';
import '../components/log';
import { consumeConfigsStore, UserConfigsStore } from '../context/user_configs';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../libs/analytics_utils';
import { BUILD_STATUS_CLASS_MAP, BUILD_STATUS_DISPLAY_MAP, BUILD_STATUS_ICON_MAP } from '../libs/constants';
import { displayDuration, NUMERIC_TIME_FORMAT } from '../libs/time_utils';
import { renderMarkdown } from '../libs/utils';
import { StepExt } from '../models/step_ext';
import './expandable_entry';
import { OnEnterList } from './lazy_list';

/**
 * Renders a step.
 */
@customElement('milo-build-step-entry')
@consumeConfigsStore
export class BuildStepEntryElement extends MobxLitElement implements OnEnterList {
  @observable.ref configsStore!: UserConfigsStore;

  @observable.ref number = 0;
  @observable.ref step!: StepExt;

  /**
   * If set to true, render a place holder until onEnterList is called.
   */
  @observable.ref prerender = false;

  @observable.ref private _expanded = false;
  get expanded() { return this._expanded; }
  set expanded(newVal) {
    this._expanded = newVal;
    // Always render the content once it was expanded so the descendants' states
    // don't get reset after the node is collapsed.
    this.shouldRenderContent = this.shouldRenderContent || newVal;
  }

  @observable.ref private shouldRenderContent = false;

  toggleAllSteps(expand: boolean) {
    this.expanded = expand;
    this.shadowRoot!.querySelectorAll<BuildStepEntryElement>('milo-build-step-entry')
      .forEach((e) => e.toggleAllSteps(expand));
  }

  onEnterList() {
    this.prerender = false;
  }

  @computed private get logs() {
    const logs = this.step.logs || [];
    return this.configsStore.userConfigs.steps.showDebugLogs
      ? logs
      : logs.filter((log) => !log.name.startsWith('$'));
  }

  private renderContent() {
    if (!this.shouldRenderContent) {
      return html``;
    }
    return html`
      <div id="summary" style=${styleMap({display: this.step.summary ? '' : 'none'})}>
        ${renderMarkdown(this.step.summary)}
      </div>
      <ul id="log-links" style=${styleMap({display: this.step.logs?.length ? '' : 'none'})}>
        ${this.logs.map((log) => html`<li><milo-log .log=${log}></li>`)}
      </ul>
      ${this.step.children?.map((child, i) => html`
      <milo-build-step-entry
        class="list-entry"
        .expanded=${!child.succeededRecursively}
        .number=${i + 1}
        .step=${child}
      ></milo-build-step-entry>
      `) || ''}
    `;
  }

  private renderDuration() {
    if (!this.step.duration) {
      return html``;
    }
    return html`
      <span id="duration"
        title="Started: ${this.step.startTime!.toFormat(NUMERIC_TIME_FORMAT)}${this.step.endTime ? `
Ended: ${this.step.endTime!.toFormat(NUMERIC_TIME_FORMAT)}` : ``}">
        ${displayDuration(this.step.duration)}
      </span>
    `;
  }

  private onMouseClick(e: MouseEvent) {
    // We need to get composedPath instead of target because if links are
    // in shadowDOM, target will be redirected
    if (e.composedPath().length === 0) {
      return;
    }
    const target = e.composedPath()[0] as Element;
    if (target.tagName === 'A') {
      const href = target.getAttribute('href') || '';
      trackEvent(GA_CATEGORIES.STEP_LINKS, GA_ACTIONS.CLICK, href);
    }
  }

  connectedCallback() {
    super.connectedCallback();
    this.addEventListener('click', this.onMouseClick);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.removeEventListener('click', this.onMouseClick);
  }

  protected render() {
    if (this.prerender) {
      return html`<div id="place-holder"></div>`;
    }

    return html`
      <milo-expandable-entry
        .expanded=${this.expanded}
        .onToggle=${(expanded: boolean) => this.expanded = expanded}
      >
        <span slot="header">
          <mwc-icon
            id="status-indicator"
            class=${BUILD_STATUS_CLASS_MAP[this.step.status]}
            title=${BUILD_STATUS_DISPLAY_MAP[this.step.status]}
          >${BUILD_STATUS_ICON_MAP[this.step.status]}</mwc-icon>
          <b>${this.number}. ${this.step.selfName}</b>
          <milo-copy-to-clipboard
            .textToCopy=${this.step.name}
            title="Copy the step name."
            @click=${(e: Event) => e.stopPropagation()}
          ></milo-copy-to-clipboard>
          <span id="header-markdown">${renderMarkdown(this.step.header)}</span>
          ${this.renderDuration()}
        </span>
        <div slot="content">${this.renderContent()}</div>
      </milo-expandable-entry>
    `;
  }

  static styles = css`
    :host {
      display: block;
    }

    #place-holder {
      height: 24px;
    }

    #status-indicator {
      vertical-align: bottom;
    }
    #status-indicator.started {
      color: var(--started-color);
    }
    #status-indicator.success {
      color: var(--success-color);
    }
    #status-indicator.failure {
      color: var(--failure-color);
    }
    #status-indicator.infra-failure {
      color: var(--critical-failure-color);
    }
    #status-indicator.canceled {
      color: var(--canceled-color);
    }

    #header-markdown * {
      display: inline;
    }

    #duration {
      color: white;
      background-color: var(--active-color);
      display: inline-block;
      padding: .25em .4em;
      font-size: 75%;
      font-weight: 700;
      line-height: 1;
      text-align: center;
      white-space: nowrap;
      vertical-align: text-bottom;
      border-radius: .25rem;
    }

    #summary {
      background-color: var(--block-background-color);
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

    #log-links>li {
      list-style-type: circle;
    }

    milo-build-step-entry {
      margin-bottom: 2px;
    }
  `;
}
