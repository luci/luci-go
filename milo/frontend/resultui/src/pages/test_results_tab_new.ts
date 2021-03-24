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

import { MobxLitElement } from '@adobe/lit-mobx';
import '@material/mwc-button';
import '@material/mwc-icon';
import { BeforeEnterObserver } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { classMap } from 'lit-html/directives/class-map';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction } from 'mobx';

import '../components/dot_spinner';
import '../components/hotkey';
import '../components/test_search_filter';
import '../components/variant_entry/index_new';
import { VariantEntryElement } from '../components/variant_entry/index_new';
import { AppState, consumeAppState } from '../context/app_state';
import { consumeInvocationState, InvocationState } from '../context/invocation_state';
import { consumeConfigsStore, UserConfigsStore } from '../context/user_configs';
import { GA_ACTIONS, GA_CATEGORIES, generateRandomLabel, trackEvent } from '../libs/analytics_utils';
import {
  VARIANT_STATUS_CLASS_MAP,
  VARIANT_STATUS_DISPLAY_MAP_TITLE_CASE,
  VARIANT_STATUS_ICON_MAP,
} from '../libs/constants';
import { TestVariant, TestVariantStatus } from '../services/resultdb';

const DEFAULT_COLUMN_WIDTH = Object.freeze<{ [key: string]: string }>({
  'v.test_suite': '350px',
});

/**
 * Display a list of test results.
 */
// TODO(crbug/1178662): replace <milo-test-results-tab> and drop the -new
// postfix.
@customElement('milo-test-results-tab-new')
@consumeInvocationState
@consumeConfigsStore
@consumeAppState
export class TestResultsTabElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref invocationState!: InvocationState;
  private enterTimestamp = 0;
  private sentLoadingTimeToGA = false;

  private disposers: Array<() => void> = [];

  async loadMore(untilStatus?: TestVariantStatus) {
    try {
      await this.invocationState.testLoader?.loadNextTestVariants(untilStatus);
    } catch (e) {
      this.dispatchEvent(
        new ErrorEvent('error', {
          error: e,
          message: e.toString(),
          composed: true,
          bubbles: true,
        })
      );
    }
  }

  @computed private get columnWidthConfig() {
    return this.invocationState.displayedColumns.map((col) => DEFAULT_COLUMN_WIDTH[col] || '100px').join(' ');
  }

  onBeforeEnter() {
    this.enterTimestamp = Date.now();
  }

  @observable.ref private allVariantsWereExpanded = false;
  private toggleAllVariants(expand: boolean) {
    this.allVariantsWereExpanded = expand;
    this.shadowRoot!.querySelectorAll<VariantEntryElement>('milo-variant-entry-new').forEach(
      (e) => (e.expanded = expand)
    );
  }
  private readonly toggleAllVariantsByHotkey = () => this.toggleAllVariants(!this.allVariantsWereExpanded);

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'test-results';
    trackEvent(GA_CATEGORIES.TEST_RESULTS_TAB, GA_ACTIONS.TAB_VISITED, window.location.href);

    // If first page of test results has already been loaded when connected
    // (happens when users switch tabs), we don't want to track the loading
    // time (only a few ms in this case)
    if (this.invocationState.testLoader?.firstPageLoaded) {
      this.sentLoadingTimeToGA = true;
    }

    // When a new test loader is received, load the first page and reset the
    // selected node.
    this.disposers.push(
      reaction(
        () => this.invocationState.testLoader,
        (testLoader) => {
          if (!testLoader) {
            return;
          }
          // The previous instance of the test results tab could've triggered
          // the loading operation already. In that case we don't want to load
          // more test results.
          if (!testLoader.firstRequestSent) {
            this.loadMore();
          }
        },
        { fireImmediately: true }
      )
    );

    // Update filters to match the querystring without saving them.
    const searchParams = new URLSearchParams(window.location.search);
    if (searchParams.has('q')) {
      this.invocationState.searchText = searchParams.get('q')!;
    }
    if (searchParams.has('clean')) {
      this.invocationState.showEmptyGroups = false;
    }
    if (searchParams.has('cols')) {
      const cols = searchParams.get('cols')!;
      this.invocationState.columnsParam = cols.split(',').filter((col) => col !== '');
    }

    // Update the querystring when filters are updated.
    this.disposers.push(
      reaction(
        () => {
          const displayedCols = this.invocationState.displayedColumns.join(',');
          const defaultCols = this.invocationState.defaultColumns.join(',');
          const newSearchParams = new URLSearchParams({
            ...(!this.invocationState.searchText ? {} : { q: this.invocationState.searchText }),
            ...(this.invocationState.showEmptyGroups ? {} : { clean: '' }),
            ...(displayedCols === defaultCols ? {} : { cols: displayedCols }),
          });
          const newSearchParamsStr = newSearchParams.toString();
          return newSearchParamsStr ? '?' + newSearchParamsStr : '';
        },
        (newQueryStr) => {
          const location = window.location;
          const newUrl = `${location.protocol}//${location.host}${location.pathname}${newQueryStr}`;
          window.history.replaceState({ path: newUrl }, '', newUrl);
        },
        { fireImmediately: true }
      )
    );
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  private renderAllVariants() {
    const testLoader = this.invocationState.testLoader;
    return html`
      ${this.renderVariants(
        TestVariantStatus.UNEXPECTED,
        testLoader?.unexpectedTestVariants || [],
        this.invocationState.showUnexpectedVariants,
        (display) => (this.invocationState.showUnexpectedVariants = display),
        testLoader?.loadedAllUnexpectedVariants || false,
        true
      )}
      ${this.renderVariants(
        TestVariantStatus.UNEXPECTEDLY_SKIPPED,
        testLoader?.unexpectedlySkippedTestVariants || [],
        this.invocationState.showUnexpectedlySkippedVariants,
        (display) => (this.invocationState.showUnexpectedlySkippedVariants = display),
        testLoader?.loadedAllUnexpectedlySkippedVariants || false
      )}
      ${this.renderVariants(
        TestVariantStatus.FLAKY,
        testLoader?.flakyTestVariants || [],
        this.invocationState.showFlakyVariants,
        (display) => (this.invocationState.showFlakyVariants = display),
        testLoader?.loadedAllFlakyVariants || false
      )}
      ${this.renderVariants(
        TestVariantStatus.EXONERATED,
        testLoader?.exoneratedTestVariants || [],
        this.invocationState.showExoneratedVariants,
        (display) => (this.invocationState.showExoneratedVariants = display),
        testLoader?.loadedAllExoneratedVariants || false
      )}
      ${this.renderVariants(
        TestVariantStatus.EXPECTED,
        testLoader?.expectedTestVariants || [],
        this.invocationState.showExpectedVariants,
        (display) => (this.invocationState.showExpectedVariants = display),
        testLoader?.loadedAllExpectedVariants || false
      )}
      <div id="variant-list-tail">
        Showing ${testLoader?.testVariantCount || 0} /
        ${testLoader?.unfilteredTestVariantCount || 0}${testLoader?.loadedAllVariants ? '' : '+'} tests.
        <span
          class="active-text"
          style=${styleMap({ display: this.invocationState.testLoader?.loadedAllVariants ?? true ? 'none' : '' })}
          >${this.renderLoadMore()}</span
        >
      </div>
    `;
  }

  private renderIntegrationHint() {
    return this.configsStore.userConfigs.hints.showTestResultsHint
      ? html`
          <div id="integration-hint">
            <p>
              Don't see results of your test framework here? This might be because they are not integrated with ResultDB
              yet. Please ask <a href="mailto: luci-eng@google.com" target="_blank">luci-eng@</a> for help.
            </p>
            <span
              class="active-text"
              @click=${() => {
                this.configsStore.userConfigs.hints.showTestResultsHint = false;
                this.configsStore.save();
              }}
              >Don't show again</span
            >
          </div>
        `
      : html`<div></div>`;
  }

  @computed private get gaLabelPrefix() {
    return 'testresults_' + this.invocationState.invocationId;
  }

  private sendLoadingTimeToGA() {
    if (this.sentLoadingTimeToGA) {
      return;
    }
    this.sentLoadingTimeToGA = true;
    trackEvent(
      GA_CATEGORIES.TEST_RESULTS_TAB,
      GA_ACTIONS.LOADING_TIME,
      generateRandomLabel(this.gaLabelPrefix),
      Date.now() - this.enterTimestamp
    );
  }

  private variantRenderedCallback = () => {
    this.sendLoadingTimeToGA();
  };

  private variantExpandedCallback = () => {
    trackEvent(GA_CATEGORIES.TEST_RESULTS_TAB, GA_ACTIONS.EXPAND_ENTRY, `${this.gaLabelPrefix}_${VISIT_ID}`, 1);
  };

  protected updated() {
    // If first page is empty, we will not get variantRenderedCallback
    if (this.invocationState.testLoader?.firstPageIsEmpty) {
      this.sendLoadingTimeToGA();
    }
  }

  private renderVariants(
    status: TestVariantStatus,
    variants: TestVariant[],
    display: boolean,
    toggleDisplay: (display: boolean) => void,
    fullyLoaded: boolean,
    expandFirst = false
  ) {
    const variantCountLabel = `${variants.length}${fullyLoaded ? '' : '+'}`;
    if (variants.length === 0 && !this.invocationState.showEmptyGroups) {
      return html``;
    }
    const firstVariant = variants[0];
    return html`
      <div
        class=${classMap({
          expanded: display,
          empty: variants.length === 0,
          'group-header': true,
        })}
        @click=${() => toggleDisplay(!display)}
      >
        <mwc-icon class="group-icon">${display ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <mwc-icon class=${'group-icon ' + VARIANT_STATUS_CLASS_MAP[status]}
          >${VARIANT_STATUS_ICON_MAP[status]}</mwc-icon
        >
        <div class="group-title">
          ${VARIANT_STATUS_DISPLAY_MAP_TITLE_CASE[status]} (${variantCountLabel})
          <span class="active-text" style=${styleMap({ display: fullyLoaded ? 'none' : '' })}
            >${this.renderLoadMore(status)}</span
          >
        </div>
      </div>
      ${repeat(
        display ? variants : [],
        (v) => `${v.testId} ${v.variantHash}`,
        (v) => html`
          <milo-variant-entry-new
            .variant=${v}
            .columnGetters=${this.invocationState.displayedColumnGetters}
            .expanded=${this.invocationState.testLoader?.testVariantCount === 1 || (v === firstVariant && expandFirst)}
            .prerender=${true}
            .renderCallback=${this.variantRenderedCallback}
            .expandedCallback=${this.variantExpandedCallback}
          ></milo-variant-entry-new>
        `
      )}
    `;
  }

  private renderLoadMore(forStatus?: TestVariantStatus) {
    const state = this.invocationState;
    return html`
      <span
        style=${styleMap({ display: state.testLoader?.isLoading ?? true ? 'none' : '' })}
        @click=${(e: Event) => {
          this.loadMore(forStatus);
          e.stopPropagation();
        }}
      >
        [load more]
      </span>
      <span
        style=${styleMap({
          display: state.testLoader?.isLoading ?? true ? '' : 'none',
          cursor: 'initial',
        })}
      >
        loading <milo-dot-spinner></milo-dot-spinner>
      </span>
    `;
  }

  private renderVariantTable() {
    const state = this.invocationState;

    if (state.invocationId === '') {
      return html`
        <div id="no-invocation">
          No associated invocation.<br />
          You need to integrate with ResultDB to see the test results.<br />
          See <a href="http://go/resultdb" target="_blank">go/resultdb</a> or ask
          <a href="mailto: luci-eng@google.com" target="_blank">luci-eng@</a> for help.
        </div>
      `;
    }

    return html`
      <div id="test-variant-table" style="--columns: ${this.columnWidthConfig}">
        <milo-hotkey
          key="space,shift+space,up,down,pageup,pagedown"
          style="display: none;"
          .handler=${() => this.shadowRoot!.getElementById('test-variant-list')!.focus()}
        ></milo-hotkey>
        <div id="table-header">
          <div><!-- Expand toggle --></div>
          <div title="variant status">&nbsp&nbspS</div>
          ${this.invocationState.displayedColumns.map((col) => html`<div title=${col}>${col.split('.', 2)[1]}</div>`)}
          <div title="test name">Name</div>
        </div>

        <milo-lazy-list id="test-variant-list" .growth=${300} tabindex="-1">${this.renderAllVariants()}</milo-lazy-list>
      </div>
    `;
  }

  protected render() {
    return html`
      <div id="header">
        <div class="filters-container">
          <input
            id="empty-groups-checkbox"
            type="checkbox"
            ?checked=${this.invocationState.showEmptyGroups}
            @change=${(e: MouseEvent) => {
              this.invocationState.showEmptyGroups = (e.target as HTMLInputElement).checked;
              this.configsStore.save();
            }}
          />
          <label for="empty-groups-checkbox" title="Show groups with no tests.">
            Empty Groups
            <mwc-icon class="inline-icon">info</mwc-icon>
          </label>
        </div>
        <div class="filters-container-delimiter"></div>
        <div id="search-label">Search:</div>
        <milo-test-search-filter></milo-test-search-filter>
        <milo-hotkey key="x" .handler=${this.toggleAllVariantsByHotkey} title="press x to expand/collapse all entries">
          <mwc-button dense unelevated @click=${() => this.toggleAllVariants(true)}>Expand All</mwc-button>
          <mwc-button dense unelevated @click=${() => this.toggleAllVariants(false)}>Collapse All</mwc-button>
        </milo-hotkey>
      </div>
      ${this.renderIntegrationHint()} ${this.renderVariantTable()}
    `;
  }

  static styles = css`
    :host {
      display: grid;
      grid-template-rows: auto auto 1fr;
      overflow-y: hidden;
    }

    #header {
      display: grid;
      grid-template-columns: auto auto auto 1fr auto;
      border-bottom: 1px solid var(--divider-color);
      grid-gap: 5px;
      height: 30px;
      padding: 5px 10px 3px 10px;
    }
    #search-label {
      margin: auto;
      padding-left: 5px;
    }
    milo-test-search-filter {
      max-width: 800px;
    }
    mwc-button {
      margin-top: 1px;
    }

    input[type='checkbox'] {
      transform: translateY(1px);
      margin-right: 3px;
    }
    .filters-container {
      display: inline-block;
      padding: 4px 5px 0;
    }
    .filters-container-delimiter {
      border-left: 1px solid var(--divider-color);
      width: 0px;
      height: 100%;
    }

    #test-variant-table {
      overflow: hidden;
      display: grid;
      grid-template-rows: auto 1fr;
    }

    #table-header {
      display: grid;
      grid-template-columns: 24px 24px var(--columns) 1fr;
      grid-gap: 5px;
      line-height: 24px;
      padding: 2px 2px 2px 10px;
      font-weight: bold;
      border-bottom: 1px solid var(--divider-color);
      background-color: var(--block-background-color);
    }

    #table-body {
      overflow-y: hidden;
    }
    milo-lazy-list > * {
      padding-left: 10px;
    }
    #no-invocation {
      padding: 10px;
    }
    #test-variant-list {
      overflow-y: auto;
      outline: none;
    }
    milo-variant-entry-new {
      margin: 2px 0px;
    }

    #integration-hint {
      border-bottom: 1px solid var(--divider-color);
      padding: 0 0 5px 15px;
    }

    .group-header {
      display: grid;
      grid-template-columns: auto auto 1fr;
      grid-gap: 5px;
      font-weight: bold;
      padding: 2px 2px 2px 10px;
      position: sticky;
      background-color: white;
      border-top: 1px solid var(--divider-color);
      top: -1px;
      cursor: pointer;
      user-select: none;
    }
    .group-title {
      line-height: 24px;
    }
    .group-header:first-child {
      top: 0px;
      border-top: none;
    }
    .group-header.expanded {
      background-color: var(--block-background-color);
    }
    .group-header.expanded:not(.empty) {
      border-bottom: 1px solid var(--divider-color);
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

    .active-text {
      color: var(--active-text-color);
      cursor: pointer;
      font-size: 14px;
      font-weight: normal;
    }
    .inline-icon {
      --mdc-icon-size: 1.2em;
      vertical-align: bottom;
    }

    #variant-list-tail {
      padding: 5px 0 5px 15px;
    }
    #variant-list-tail:not(:first-child) {
      border-top: 1px solid var(--divider-color);
    }
    #load {
      color: var(--active-text-color);
    }
  `;
}
