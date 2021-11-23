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

import '../../expandable_entry';
import '../../copy_to_clipboard';
import './result_entry';
import { AppState, consumeAppState } from '../../../context/app_state';
import { GA_ACTIONS, GA_CATEGORIES, generateRandomLabel, trackEvent } from '../../../libs/analytics_utils';
import { VARIANT_STATUS_CLASS_MAP, VARIANT_STATUS_ICON_MAP } from '../../../libs/constants';
import { lazyRendering, RenderPlaceHolder } from '../../../libs/observer_element';
import { sanitizeHTML } from '../../../libs/sanitize_html';
import { TestVariant, TestVariantStatus } from '../../../services/resultdb';
import colorClasses from '../../../styles/color_classes.css';
import commonStyle from '../../../styles/common_style.css';

// This list defines the order in which variant def keys should be displayed.
// Any unrecognized keys will be listed after the ones defined below.
const ORDERED_VARIANT_DEF_KEYS = Object.freeze(['bucket', 'builder', 'test_suite']);

// Only track test variants with unexpected, non-exonerated test results.
const TRACKED_STATUS = [TestVariantStatus.UNEXPECTED, TestVariantStatus.UNEXPECTEDLY_SKIPPED, TestVariantStatus.FLAKY];

/**
 * Renders an expandable entry of the given test variant.
 */
@customElement('milo-test-variant-entry')
@lazyRendering
export class TestVariantEntryElement extends MobxLitElement implements RenderPlaceHolder {
  @observable.ref @consumeAppState() appState!: AppState;

  @observable.ref variant!: TestVariant;
  @observable.ref columnGetters: Array<(v: TestVariant) => unknown> = [];

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
      trackEvent(GA_CATEGORIES.TEST_RESULTS_TAB, GA_ACTIONS.EXPAND_ENTRY, VISIT_ID, 1);
    }
  }

  @observable.ref private shouldRenderContent = false;
  private rendered = false;

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

  private genTestLink() {
    const location = window.location;
    const query = new URLSearchParams(location.search);
    query.set('q', `ExactID:${this.variant.testId} VHash:${this.variant.variantHash}`);
    return `${location.protocol}//${location.host}${location.pathname}?${query}`;
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

  private trackInteraction = () => {
    if (TRACKED_STATUS.includes(this.variant.status)) {
      trackEvent(GA_CATEGORIES.TEST_VARIANT_WITH_UNEXPECTED_RESULTS, GA_ACTIONS.INSPECT_TEST, VISIT_ID);
    }
  };

  connectedCallback() {
    super.connectedCallback();
    this.addEventListener('click', this.trackInteraction);
  }

  disconnectedCallback() {
    this.removeEventListener('click', this.trackInteraction);
    super.disconnectedCallback();
  }

  private renderBody() {
    if (!this.shouldRenderContent) {
      return html``;
    }
    return html`
      <div id="basic-info">
        <a href=${this.sourceUrl} target="_blank" style=${styleMap({ display: this.sourceUrl ? '' : 'none' })}
          >source</a
        >
        ${this.sourceUrl ? '|' : ''}
        <div id="test-id">
          <span class="greyed-out" title=${this.variant.testId}>ID: ${this.variant.testId}</span>
          <milo-copy-to-clipboard
            .textToCopy=${this.variant.testId}
            @click=${(e: Event) => {
              e.stopPropagation();
              this.trackInteraction();
            }}
            title="copy test ID to clipboard"
          ></milo-copy-to-clipboard>
        </div>
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
      </div>
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

  renderPlaceHolder() {
    return '';
  }

  protected render() {
    this.rendered = true;
    return html`
      <milo-expandable-entry .expanded=${this.expanded} .onToggle=${(expanded: boolean) => (this.expanded = expanded)}>
        <div id="header" slot="header">
          <mwc-icon class=${VARIANT_STATUS_CLASS_MAP[this.variant.status]}>
            ${VARIANT_STATUS_ICON_MAP[this.variant.status]}
          </mwc-icon>
          ${this.columnValues.map((v) => html`<div title=${v}>${v}</div>`)}
          <div id="test-name">
            <span title=${this.longName}>${this.shortName}</span>
            <milo-copy-to-clipboard
              .textToCopy=${this.longName}
              @click=${(e: Event) => {
                e.stopPropagation();
                this.trackInteraction();
              }}
              title="copy test name to clipboard"
            ></milo-copy-to-clipboard>
            <milo-copy-to-clipboard
              id="link-copy-button"
              .textToCopy=${() => this.genTestLink()}
              @click=${(e: Event) => {
                e.stopPropagation();
                this.trackInteraction();
              }}
              title="copy link to the test"
            >
              <mwc-icon slot="copy-icon">link</mwc-icon>
            </milo-copy-to-clipboard>
          </div>
        </div>
        <div id="body" slot="content">${this.renderBody()}</div>
      </milo-expandable-entry>
    `;
  }

  protected updated() {
    if (!this.rendered || this.appState.sentTestResultsTabLoadingTimeToGA) {
      return;
    }
    this.appState.sentTestResultsTabLoadingTimeToGA = true;
    trackEvent(
      GA_CATEGORIES.TEST_RESULTS_TAB,
      GA_ACTIONS.LOADING_TIME,
      generateRandomLabel(VISIT_ID + '_'),
      Date.now() - this.appState.tabSelectionTime
    );
  }

  static styles = [
    commonStyle,
    colorClasses,
    css`
      :host {
        display: block;
        min-height: 24px;
      }

      #header {
        display: grid;
        grid-template-columns: 24px var(--tvt-columns) 1fr;
        grid-gap: 5px;
        font-size: 16px;
        line-height: 24px;
      }
      #header > * {
        overflow: hidden;
        text-overflow: ellipsis;
      }

      #test-name {
        display: flex;
        font-size: 16px;
        line-height: 24px;
      }
      #test-name > span {
        overflow: hidden;
        text-overflow: ellipsis;
      }

      #body {
        overflow: hidden;
      }

      #basic-info {
        font-weight: 500;
        line-height: 24px;
        margin-left: 5px;
      }

      #test-id {
        display: inline-flex;
        max-width: 300px;
        overflow: hidden;
        white-space: nowrap;
      }
      #test-id > span {
        display: inline-block;
        overflow: hidden;
        text-overflow: ellipsis;
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
        flex: 0 0 16px;
        margin-left: 2px;
        display: none;
      }
      :hover > milo-copy-to-clipboard {
        display: inline-block;
      }
    `,
  ];
}
