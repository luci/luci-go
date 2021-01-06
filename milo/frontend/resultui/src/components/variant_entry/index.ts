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
import { classMap } from 'lit-html/directives/class-map';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import takeWhile from 'lodash-es/takeWhile';
import { computed, observable } from 'mobx';
import { VARIANT_STATUS_CLASS_MAP, VARIANT_STATUS_DISPLAY_MAP, VARIANT_STATUS_ICON_MAP } from '../../libs/constants';

import { sanitizeHTML } from '../../libs/sanitize_html';
import { ID_SEG_REGEX, ReadonlyVariant } from '../../models/test_node';
import '../copy_to_clipboard';
import './result_entry';

// This list defines the order in which variant def keys should be displayed.
// Any unrecognized keys will be listed after the ones defined below.
const ORDERED_VARIANT_DEF_KEYS = Object.freeze([
  'bucket',
  'builder',
  'test_suite',
]);

/**
 * Renders an expandable entry of the given test variant.
 */
@customElement('milo-variant-entry')
export class VariantEntryElement extends MobxLitElement {
  @observable.ref variant!: ReadonlyVariant;
  @observable.ref prevTestId = '';
  @observable.ref prevVariant?: ReadonlyVariant;
  @observable.ref displayVariantId = true;

  @observable.ref private _expanded = false;
  @computed get expanded() {
    return this._expanded;
  }
  set expanded(newVal: boolean) {
    this._expanded = newVal;
    // Always render the content once it was expanded so the descendants' states
    // don't get reset after the node is collapsed.
    this.shouldRenderContent = this.shouldRenderContent || newVal;
  }

  @observable.ref private shouldRenderContent = false;

  /**
   * Common prefix between this.variant.testId and this.prevTestId.
   */
  @computed
  private get commonTestIdPrefix() {
    const prevSegs = this.prevTestId.match(ID_SEG_REGEX)!;
    const currentSegs = this.variant.testId.match(ID_SEG_REGEX)!;
    return takeWhile(prevSegs, (seg, i) => currentSegs[i] === seg).join('');
  }

  @computed
  private get hasSingleChild() {
    return (this.variant!.results.length + this.variant!.exonerations.length) === 1;
  }

  @computed
  private get variantDef() {
    const def = this.variant!.variant.def;
    const res: Array<[string, string]> = [];
    const seen = new Set();
    for (const key of ORDERED_VARIANT_DEF_KEYS) {
      if (def.hasOwnProperty(key)) {
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
    return this.variant!.results.findIndex((e) => !e.expected);
  }

  private renderBody() {
    if (!this.shouldRenderContent) {
      return html``;
    }
    return html`
      <div id="content-ruler"></div>
      <div id="content" style=${styleMap({display: this.expanded ? '' : 'none'})}>
        <span id="variant-def">
          <span
            class=${VARIANT_STATUS_CLASS_MAP[this.variant.status]}
          >${VARIANT_STATUS_DISPLAY_MAP[this.variant.status]} result</span>
          ${this.variantDef.length === 0 ? '' : '|'}
          <span class="greyed-out">
            ${this.variantDef.map(([k, v]) => html`
            <span class="kv">
              <span class="kv-key">${k}</span>
              <span class="kv-value">${v}</span>
            </span>
            `)}
          </span>
        </span>
        ${repeat(this.variant!.exonerations, (e) => e.exonerationId, (e) => html`
        <div class="explanation-html">
          ${sanitizeHTML(e.explanationHtml || 'This test variant had unexpected results, but was exonerated (reason not provided).')}
        </div>
        `)}
        ${repeat(this.variant!.results, (r) => r.resultId, (r, i) => html`
        <milo-result-entry
          .id=${i + 1}
          .testResult=${r}
          .expanded=${i === this.expandedResultIndex}
        ></milo-result-entry>
        `)}
      </div>
    `;
  }

  protected render() {
    return html`
      <div>
        <div
          class=${classMap({'expanded': this.expanded, 'display-variant-id': this.displayVariantId, 'expandable-header': true})}
          @click=${() => this.expanded = !this.expanded}
        >
          <mwc-icon id="expand-toggle">${this.expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
          <div id="header" class="one-line-content">
            <mwc-icon
              id="status-indicator"
              class=${classMap({[VARIANT_STATUS_CLASS_MAP[this.variant.status]]: true})}
            >${VARIANT_STATUS_ICON_MAP[this.variant.status]}</mwc-icon>
            <div id="identifier">
              <div id="test-identifier">
                <span>
                  <span class="greyed-out">${this.commonTestIdPrefix}</span>${this.variant.testId.slice(this.commonTestIdPrefix.length)}
                </span>
                <milo-copy-to-clipboard
                  .textToCopy=${this.variant.testId}
                  @click=${(e: Event) => e.stopPropagation()}
                  title="copy test ID to clipboard"
                ></milo-copy-to-clipboard>
              </div>
              <div id="variant-identifier">
                <span>
                  ${this.variantDef.map(([k, v]) => html`
                  <span class=${classMap({'greyed-out': !this.prevVariant || v === this.prevVariant.variant.def?.[k], 'kv': true})}>
                    <span class="kv-key">${k}</span>
                    <span class="kv-value">${v}</span>
                  </span>
                  `)}
                </span>
              </div>
            </div>
          </div>
        </div>
        <div id="body">${this.renderBody()}</div>
      </div>
    `;
  }

  static styles = css`
    :host {
      display: block;
    }

    .expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      letter-spacing: 0.15px;
      grid-gap: 5px;
      cursor: pointer;
      user-select: none;
    }
    .expandable-header.display-variant-id:not(.expanded) {
      grid-template-rows: 32px;
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

    #header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 36px;
      grid-gap: 5px;
    }
    #status-indicator {
      grid-row: 1;
      grid-column: 1;
    }
    .exonerated {
      color: var(--exonerated-color);
    }
    .expected {
      color: var(--success-color);
    }
    .unexpected {
      color: var(--failure-color);
    }
    .flaky {
      color: var(--warning-color);
    }
    #identifier {
      overflow: hidden;
      grid-row: 1;
      grid-column: 2;
    }
    #test-identifier {
      display: flex;
      overflow: hidden;
      font-size: 16px;
      line-height: 24px;
    }
    #test-identifier>span {
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .expandable-header.display-variant-id:not(.expanded) #test-identifier {
      line-height: 16px;
    }
    #variant-identifier {
      display: none;
      font-size: 12px;
      line-height: 12px;
    }
    .expandable-header.display-variant-id:not(.expanded) #variant-identifier {
      display: block;
    }

    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
    }
    #content {
      overflow: hidden;
    }
    #content-ruler {
      border-left: 1px solid var(--divider-color);
      width: 0px;
      margin-left: 11.5px;
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
    .kv:last-child>.kv-value::after {
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
      margin-left: 5px;
      margin-right: 5px;
    }
  `;
}
