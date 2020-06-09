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
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import { AppState, consumeAppState } from '../../context/app_state_provider';
import { sanitizeHTML } from '../../libs/sanitize_html';
import { ListArtifactsResponse, TestResult, TestStatus } from '../../services/resultdb';
import './text_diff_artifacts';

const STATUS_DISPLAY_MAP = {
  [TestStatus.Unspecified]: 'unspecified',
  [TestStatus.Pass]: 'passed',
  [TestStatus.Fail]: 'failed',
  [TestStatus.Skip]: 'skipped',
  [TestStatus.Crash]: 'crashed',
  [TestStatus.Abort]: 'aborted',
};


/**
 * Renders an expandable entry of the given test result.
 */
export class ResultEntryElement extends MobxLitElement {
  @observable.ref id = '';
  @observable.ref testResult!: TestResult;
  @observable.ref appState!: AppState;

  @observable.ref private wasExpanded = false;
  @observable.ref private _expanded = false;
  @computed get expanded() {
    return this._expanded;
  }
  set expanded(newVal: boolean) {
    this._expanded = newVal;
    this.wasExpanded = this.wasExpanded || newVal;
  }

  @observable.ref private artifactsExpanded = false;
  @observable.ref private tagExpanded = false;

  @computed
  private get artifactsRes(): IPromiseBasedObservable<ListArtifactsResponse> {
    if (!this.wasExpanded || !this.appState.resultDb) {
      // Returns a promise that never resolves when
      //  - resultDb isn't ready.
      //  - or the entry was never expanded.
      return fromPromise(new Promise(() => {}));
    }
    // TODO(weiweilin): handle pagination.
    return fromPromise(this.appState.resultDb.listArtifacts({parent: this.testResult.name}));
  }

  @computed private get artifacts() { return this.artifactsRes.state === 'fulfilled' ? this.artifactsRes.value.artifacts || [] : []; }

  @computed private get textDiffArtifacts() {
    return this.artifacts.filter((a) => a.artifactId.match(/_diff$/) && a.contentType === 'text/plain');
  }

  private renderSummaryHtml() {
    if (!this.testResult.summaryHtml) {
      return html``;
    }

    return html`
      <div id="summary-html">
        ${sanitizeHTML(this.testResult.summaryHtml)}
      </div>
    `;
  }

  private renderTags() {
    if (this.testResult.tags.length === 0) {
      return html``;
    }

    return html`
      <div
        class="expandable-header"
        @click=${() => this.tagExpanded = !this.tagExpanded}
      >
        <mwc-icon class="expand-toggle">${this.tagExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <span class="one-line-content">
          Tags:
          <span class="light" style=${styleMap({display: this.tagExpanded ? 'none': ''})}>
            ${this.testResult.tags.map((tag) => html`
            <span class="kv-key">${tag.key}</span>
            <span class="kv-value">${tag.value}</span>
            `)}
          </span>
        </span>
      </div>
      <table id="tag-table" border="0" style=${styleMap({display: this.tagExpanded ? '': 'none'})}>
        ${this.testResult.tags.map((tag) => html`
        <tr>
          <td>${tag.key}:</td>
          <td>${tag.value}</td>
        </tr>
        `)}
      </table>
    `;
  }

  private renderArtifacts() {
    if (this.artifacts.length == 0) {
      return html``;
    }

    return html`
      <div
        class="expandable-header"
        @click=${() => this.artifactsExpanded = !this.artifactsExpanded}
      >
        <mwc-icon class="expand-toggle">${this.artifactsExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <div class="one-line-content">
          Artifacts:
          <span class="light">${this.artifacts.length}</span>
        </div>
      </div>
      <ul id="artifact-list" style=${styleMap({display: this.artifactsExpanded ? '' : 'none'})}>
        ${this.artifacts.map((artifact) => html`
        <!-- TODO(weiweilin): refresh when the fetchUrl expires -->
        <li><a href=${artifact.fetchUrl}>${artifact.artifactId}</a></li>
        `)}
      </ul>
    `;
  }

  protected render() {
    return html`
      <div>
        <div
          id="entry-header"
          class="expandable-header"
          @click=${() => this.expanded = !this.expanded}
        >
          <mwc-icon class="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
          <span class="one-line-content">
            run #${this.id}
            <span class="${this.testResult.expected ? 'expected' : 'unexpected'}-result">
              ${this.testResult.expected ? '' : html`unexpectedly`}
              ${STATUS_DISPLAY_MAP[this.testResult.status]}
            </span>
            ${this.testResult.duration ? `after ${this.testResult.duration}` : ''}
          </span>
        </div>
        <div id="body">
          <div id="content-ruler"></div>
          <div id="content" style=${styleMap({display: this.expanded ? '' : 'none'})}>
            ${this.renderSummaryHtml()}
            ${this.textDiffArtifacts.map((artifact) => html`
            <tr-text-diff-artifact .artifact=${artifact}>
            </tr-text-diff-artifact>
            `)}
            ${this.renderTags()}
            ${this.renderArtifacts()}
          </div>
        </div>
      </div>
    `;
  }

  static styles = css`
    .expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      grid-gap: 5px;
      cursor: pointer;
    }
    .expandable-header .expand-toggle {
      grid-row: 1;
      grid-column: 1;
    }
    .expandable-header .one-line-content {
      grid-row: 1;
      grid-column: 2;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }
    #entry-header .one-line-content {
      font-size: 14px;
      letter-spacing: 0.1px;
      font-weight: 500;
    }

    .expected-result {
      color: rgb(51, 172, 113);
    }
    .unexpected-result {
      color: rgb(210, 63, 49);
    }

    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
    }
    #content-ruler {
      border-left: 1px solid #DDDDDD;
      width: 0px;
      margin-left: 11.5px;
    }
    #summary-html {
      background-color: rgb(245, 245, 245);
      padding: 5px;
    }
    #summary-html pre {
      margin: 0;
      font-size: 12px;
    }

    .kv-key::after {
      content: ':';
    }
    .kv-value::after {
      content: ',';
    }
    .kv-value:last-child::after {
      content: '';
    }
    .light {
      color: grey;
    }

    #tag-table {
      margin-left: 29px;
    }

    ul {
      margin: 3px 0;
      padding-inline-start: 28px;
    }
    `;
}

customElement('tr-result-entry')(
  consumeAppState(ResultEntryElement),
);
