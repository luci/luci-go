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
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../libs/analytics_utils';
import { TestVariant, TestVariantStatus } from '../services/resultdb';

const DEFAULT_COLUMN_WIDTH = Object.freeze<{ [key: string]: string }>({
  'v.test_suite': '350px',
});

function getColumnLabel(key: string) {
  // If the key has the format of '{type}.{value}', hide the '{type}.' prefix.
  return key.split('.', 2)[1] ?? key;
}

/**
 * Display a list of test results.
 */
// TODO(crbug/1178662): replace <milo-test-results-tab> and drop the -new
// postfix.
@customElement('milo-test-results-tab-new')
@consumeInvocationState
@consumeConfigsStore
@consumeAppState
export class TestResultsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref invocationState!: InvocationState;

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
    // TODO(weiweilin): track test results tab loading time.

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
    if (searchParams.has('sortby')) {
      const sortingKeys = searchParams.get('sortby')!;
      this.invocationState.sortingKeysParam = sortingKeys.split(',').filter((col) => col !== '');
    }
    if (searchParams.has('groupby')) {
      const groupingKeys = searchParams.get('groupby')!;
      this.invocationState.groupingKeysParam = groupingKeys.split(',').filter((key) => key !== '');
    }

    // Update the querystring when filters are updated.
    this.disposers.push(
      reaction(
        () => {
          const displayedCols = this.invocationState.displayedColumns.join(',');
          const defaultCols = this.invocationState.defaultColumns.join(',');
          const sortingKeys = this.invocationState.sortingKeys.join(',');
          const defaultSortingKeys = this.invocationState.defaultSortingKeys.join(',');
          const groupingKeys = this.invocationState.groupingKeys.join(',');
          const defaultGroupingKeys = this.invocationState.defaultGroupingKeys.join(',');

          const newSearchParams = new URLSearchParams({
            ...(!this.invocationState.searchText ? {} : { q: this.invocationState.searchText }),
            ...(this.invocationState.showEmptyGroups ? {} : { clean: '' }),
            ...(displayedCols === defaultCols ? {} : { cols: displayedCols }),
            ...(sortingKeys === defaultSortingKeys ? {} : { sortby: sortingKeys }),
            ...(groupingKeys === defaultGroupingKeys ? {} : { groupby: groupingKeys }),
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
    const groupers = this.invocationState.groupers;
    return html`
      ${(testLoader?.groupedNonExpectedVariants || []).map((group) =>
        this.renderVariantGroup(
          groupers.map(([key, getter]) => [key, getter(group[0])]),
          group
        )
      )}
      ${this.renderVariantGroup([['status', TestVariantStatus.EXPECTED]], testLoader?.expectedTestVariants || [])}
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

  private variantExpandedCallback = () => {
    trackEvent(GA_CATEGORIES.TEST_RESULTS_TAB, GA_ACTIONS.EXPAND_ENTRY, `${this.gaLabelPrefix}_${VISIT_ID}`, 1);
  };

  @observable private collapsedVariantGroups = new Set<string>();
  private renderVariantGroup(groupDef: [string, unknown][], variants: TestVariant[]) {
    const groupId = JSON.stringify(groupDef);
    const expanded = !this.collapsedVariantGroups.has(groupId);
    return html`
      <div
        class=${classMap({
          expanded,
          empty: variants.length === 0,
          'group-header': true,
        })}
        @click=${() => {
          if (expanded) {
            this.collapsedVariantGroups.add(groupId);
          } else {
            this.collapsedVariantGroups.delete(groupId);
          }
        }}
      >
        <mwc-icon class="group-icon">${expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <div>
          <b>${variants.length} test variant${variants.length === 1 ? '' : 's'}:</b>
          ${groupDef.map(([k, v]) => html`<span class="group-kv"><span>${getColumnLabel(k)}=</span><b>${v}</b></span>`)}
        </div>
      </div>
      ${repeat(
        expanded ? variants : [],
        (v) => `${v.testId} ${v.variantHash}`,
        (v) => html`
          <milo-variant-entry-new
            .variant=${v}
            .columnGetters=${this.invocationState.displayedColumnGetters}
            .expanded=${this.invocationState.testLoader?.testVariantCount === 1}
            .prerender=${true}
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
          ${this.invocationState.displayedColumns.map((col) => html`<div title=${col}>${getColumnLabel(col)}</div>`)}
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
      padding: 2px 2px 2px 10px;
      position: sticky;
      font-size: 14px;
      background-color: var(--block-background-color);
      border-top: 1px solid var(--divider-color);
      top: -1px;
      cursor: pointer;
      user-select: none;
      line-height: 24px;
    }
    .group-header:first-child {
      top: 0px;
      border-top: none;
    }
    .group-header.expanded:not(.empty) {
      border-bottom: 1px solid var(--divider-color);
    }
    .group-kv:not(:last-child)::after {
      content: ', ';
    }
    .group-kv > span {
      color: var(--light-text-color);
    }
    .group-kv > b {
      font-style: italic;
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
