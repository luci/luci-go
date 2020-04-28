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
import { observable } from 'mobx';

import { sanitizeHTML } from '../../libs/sanitize_html';
import { TestResult, TestStatus } from '../../services/resultdb';

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
@customElement('tr-result-entry')
export class ResultEntryElement extends MobxLitElement {
  @observable.ref id = '';
  @observable.ref testResult?: TestResult;
  @observable.ref expanded = false;

  @observable.ref private outputArtifactsExpanded = true;
  @observable.ref private inputArtifactsExpanded = false;
  @observable.ref private tagExpanded = false;

  private renderSummaryHtml() {
    if (!this.testResult!.summaryHtml) {
      return html``;
    }

    return html`
      <div id="summary-html">
        ${sanitizeHTML(this.testResult!.summaryHtml)}
      </div>
    `;
  }

  private renderTags() {
    if (this.testResult!.tags.length === 0) {
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
            ${this.testResult!.tags.map((tag) => html`
            <span class="kv-key">${tag.key}</span>
            <span class="kv-value">${tag.value}</span>
            `)}
          </span>
        </span>
      </div>
      <table id="tag-table" border="0" style=${styleMap({display: this.tagExpanded ? '': 'none'})}>
        ${this.testResult!.tags.map((tag) => html`
        <tr>
          <td>${tag.key}:</td>
          <td>${tag.value}</td>
        </tr>
        `)}
      </table>
    `;
  }

  private renderOutputArtifacts() {
    if (!this.testResult!.outputArtifacts?.length) {
      return html``;
    }

    return html`
      <div
        class="expandable-header"
        @click=${() => this.outputArtifactsExpanded = !this.outputArtifactsExpanded}
      >
        <mwc-icon class="expand-toggle">${this.outputArtifactsExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <div class="one-line-content">
          Output Artifacts:
          <span class="light">${this.testResult!.outputArtifacts?.length || 0} artifact(s)</span>
        </div>
      </div>
      <ul style=${styleMap({display: this.outputArtifactsExpanded ? '' : 'none'})}>
        ${this.testResult!.outputArtifacts?.map((artifact) => html`
        <!-- TODO(weiweilin): refresh when the fetchUrl expires -->
        <li><a href=${artifact.fetchUrl}>${artifact.name}</a></li>
        `)}
      </ul>
    `;
  }

  private renderInputArtifacts() {
    if (!this.testResult?.inputArtifacts?.length) {
      return html``;
    }

    return html`
      <div
        class="expandable-header"
        @click=${() => this.inputArtifactsExpanded = !this.inputArtifactsExpanded}
      >
        <mwc-icon class="expand-toggle">${this.inputArtifactsExpanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <div class="one-line-content">
          Input Artifacts:
          <span class="light">${this.testResult!.inputArtifacts?.length || 0} artifact(s)</span>
        </div>
      </div>
      <ul style=${styleMap({display: this.inputArtifactsExpanded ? '' : 'none'})}>
        ${this.testResult!.inputArtifacts?.map((artifact) => html`
        <!-- TODO(weiweilin): refresh when the fetchUrl expires -->
        <li><a href=${artifact.fetchUrl}>${artifact.name}</a></li>
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
            <span class="${this.testResult!.expected ? 'expected' : 'unexpected'}-result">
              ${this.testResult!.expected ? '' : html`unexpectedly`}
              ${STATUS_DISPLAY_MAP[this.testResult!.status]}
            </span> after ${this.testResult!.duration || '-s'}
          </span>
        </div>
        <div id="body">
          <div id="content-ruler"></div>
          <div id="content" style=${styleMap({display: this.expanded ? '' : 'none'})}>
            ${this.renderSummaryHtml()}
            ${this.renderTags()}
            ${this.renderOutputArtifacts()}
            ${this.renderInputArtifacts()}
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
    #summary-html ul {
      margin: 3px 0;
      padding-inline-start: 28px;
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
    `;
}
