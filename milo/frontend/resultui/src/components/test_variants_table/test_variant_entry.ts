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

import '@material/mwc-icon';
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';

import '../expandable_entry';
import '../lazy_list';
import '../copy_to_clipboard';
import '../variant_entry/result_entry';
import { VARIANT_STATUS_CLASS_MAP, VARIANT_STATUS_DISPLAY_MAP, VARIANT_STATUS_ICON_MAP } from '../../libs/constants';
import { sanitizeHTML } from '../../libs/sanitize_html';
import { TestVariant } from '../../services/resultdb';
import { OnEnterList } from '../lazy_list';

// This list defines the order in which variant def keys should be displayed.
// Any unrecognized keys will be listed after the ones defined below.
const ORDERED_VARIANT_DEF_KEYS = Object.freeze(['bucket', 'builder', 'test_suite']);

/**
 * Renders an expandable entry of the given test variant.
 */
@customElement('milo-test-variant-entry')
export class TestVariantEntryElement extends MobxLitElement implements OnEnterList {
  @observable.ref variant!: TestVariant;
  @observable.ref columnGetters: Array<(v: TestVariant) => unknown> = [];
  @observable.ref expandedCallback = () => {};
  @observable.ref renderedCallback = () => {};

  @observable.ref private _expanded = false;
  @computed get expanded() {
    return this._expanded;
  }
  set expanded(newVal: boolean) {
    this._expanded = newVal;
    // Always render the content once it was expanded so the descendants' states
    // don't get reset after the node is collapsed.
    this.shouldRenderContent = this.shouldRenderContent || newVal;

    if (newVal) {
      this.expandedCallback();
    }
  }

  /**
   * If set to true, render a place holder until onEnterList is called.
   */
  @observable.ref prerender = false;

  onEnterList() {
    this.prerender = false;
  }

  @observable.ref private shouldRenderContent = false;

  @computed
  private get shortName() {
    if (this.variant.testMetadata?.name) {
      return this.variant.testMetadata.name;
    }

    // Generate a good enough short name base on the test ID.
    const suffix = this.variant.testId.match(/^.*[./]([^./]*?.{40})$/);
    if (suffix) {
      return '...' + suffix[1];
    }
    return this.variant.testId;
  }

  @computed
  private get longName() {
    if (this.variant.testMetadata?.name) {
      return this.variant.testMetadata.name;
    }
    return this.variant.testId;
  }

  @computed
  private get sourceUrl() {
    const testLocation = this.variant.testMetadata?.location;
    if (!testLocation) {
      return null;
    }
    return (
      testLocation.repo +
      '/+/HEAD' +
      testLocation.fileName.slice(1) +
      (testLocation.line ? '#' + testLocation.line : '')
    );
  }

  @computed
  private get hasSingleChild() {
    return (this.variant.results?.length ?? 0) + (this.variant.exonerations?.length ?? 0) === 1;
  }

  @computed
  private get variantDef() {
    const def = this.variant!.variant?.def || {};
    const res: Array<[string, string]> = [];
    const seen = new Set();
    for (const key of ORDERED_VARIANT_DEF_KEYS) {
      if (Object.prototype.hasOwnProperty.call(def, key)) {
        res.push([key, def[key]]);
        seen.add(key);
      }
    }
    for (const [key, value] of Object.entries(def)) {
      if (!seen.has(key)) {
        res.push([key, value]);
      }
    }
    return res;
  }

  @computed
  private get expandedResultIndex() {
    // If there's only a single result, just expand it (even if it passed).
    if (this.hasSingleChild) {
      return 0;
    }
    // Otherwise expand the first failed result, or -1 if there aren't any.
    return this.variant.results?.findIndex((e) => !e.result.expected) ?? -1;
  }

  @computed get columnValues() {
    return this.columnGetters.map((fn) => fn(this.variant));
  }

  private renderBody() {
    if (!this.shouldRenderContent) {
      return html``;
    }
    return html`
      <span id="variant-def">
        <span class=${VARIANT_STATUS_CLASS_MAP[this.variant.status]}>
          ${VARIANT_STATUS_DISPLAY_MAP[this.variant.status]} result
        </span>
        ${this.sourceUrl ? '|' : ''}
        <a href=${this.sourceUrl} target="_blank" style=${styleMap({ display: this.sourceUrl ? '' : 'none' })}
          >source</a
        >
        ${this.variantDef.length !== 0 ? '|' : ''}
        <span class="greyed-out">
          ${this.variantDef.map(
            ([k, v]) => html`
              <span class="kv">
                <span class="kv-key">${k}</span>
                <span class="kv-value">${v}</span>
              </span>
            `
          )}
        </span>
      </span>
      ${repeat(
        this.variant.exonerations || [],
        (e) => e.exonerationId,
        (e) => html`
          <div class="explanation-html">
            ${sanitizeHTML(
              e.explanationHtml || 'This test variant had unexpected results, but was exonerated (reason not provided).'
            )}
          </div>
        `
      )}
      ${repeat(
        this.variant.results || [],
        (r) => r.result.resultId,
        (r, i) => html`
          <milo-result-entry
            .id=${i + 1}
            .testResult=${r.result}
            .expanded=${i === this.expandedResultIndex}
          ></milo-result-entry>
        `
      )}
    `;
  }

  protected render() {
    if (this.prerender) {
      return html`<div id="place-holder"></div>`;
    }

    return html`
      <milo-expandable-entry .expanded=${this.expanded} .onToggle=${(expanded: boolean) => (this.expanded = expanded)}>
        <div id="header" slot="header">
          <mwc-icon class=${VARIANT_STATUS_CLASS_MAP[this.variant.status]}>
            ${VARIANT_STATUS_ICON_MAP[this.variant.status]}
          </mwc-icon>
          ${this.columnValues.map((v) => html`<div title=${v}>${v}</div>`)}
          <div id="test-identifier">
            <span title=${this.longName}>${this.shortName}</span>
            <milo-copy-to-clipboard
              .textToCopy=${this.longName}
              @click=${(e: Event) => e.stopPropagation()}
              title="copy test name to clipboard"
            ></milo-copy-to-clipboard>
          </div>
        </div>
        <div id="body" slot="content">${this.renderBody()}</div>
      </milo-expandable-entry>
    `;
  }

  protected updated() {
    if (!this.prerender) {
      this.renderedCallback();
    }
  }

  static styles = css`
    :host {
      display: block;
    }

    #place-holder {
      height: 24px;
    }

    #header {
      display: grid;
      grid-template-columns: 24px var(--columns) 1fr;
      grid-gap: 5px;
      user-select: none;
      font-size: 16px;
      line-height: 24px;
    }
    #header > * {
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .unexpected {
      color: var(--failure-color);
    }
    .unexpectedly-skipped {
      color: var(--critical-failure-color);
    }
    .flaky {
      color: var(--warning-color);
    }
    span.flaky {
      color: var(--warning-text-color);
    }
    .exonerated {
      color: var(--exonerated-color);
    }
    .expected {
      color: var(--success-color);
    }
    #test-identifier {
      display: flex;
      font-size: 16px;
      line-height: 24px;
    }
    #test-identifier > span {
      overflow: hidden;
      text-overflow: ellipsis;
    }
    milo-copy-to-clipboard {
      flex: 0 0 16px;
    }

    #body {
      overflow: hidden;
    }

    #variant-def {
      font-weight: 500;
      line-height: 24px;
      margin-left: 5px;
    }
    .kv-key::after {
      content: ':';
    }
    .kv-value::after {
      content: ',';
    }
    .kv:last-child > .kv-value::after {
      content: '';
    }
    #def-table {
      margin-left: 29px;
    }

    .greyed-out {
      color: var(--greyed-out-text-color);
    }

    .explanation-html {
      background-color: var(--block-background-color);
      padding: 5px;
    }

    milo-copy-to-clipboard {
      visibility: hidden;
      margin-left: 5px;
      margin-right: 5px;
    }
    #header:hover milo-copy-to-clipboard {
      visibility: visible;
    }
  `;
}
