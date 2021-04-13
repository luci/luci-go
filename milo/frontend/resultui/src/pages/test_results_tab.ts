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
import '../components/variant_entry';
import { MiloBaseElement } from '../components/milo_base';
import { VariantEntryElement } from '../components/variant_entry';
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
import colorClasses from '../styles/color_classes.css';
import commonStyle from '../styles/common_style.css';

/**
 * Display a list of test results.
 */
// TODO(crbug/1178662): replace this with <milo-test-results-tab-new>.
// This component should be in feature freeze.
@customElement('milo-test-results-tab')
@consumeInvocationState
@consumeConfigsStore
@consumeAppState
export class TestResultsTabElement extends MiloBaseElement {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref invocationState!: InvocationState;
  private sentLoadingTimeToGA = false;

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

  @observable.ref private allVariantsWereExpanded = false;
  private toggleAllVariants(expand: boolean) {
    this.allVariantsWereExpanded = expand;
    this.shadowRoot!.querySelectorAll<VariantEntryElement>('milo-variant-entry').forEach((e) => (e.expanded = expand));
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
    this.addDisposer(
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

    // Update the querystring when filters are updated.
    this.addDisposer(
      reaction(
        () => {
          const newSearchParams = new URLSearchParams({
            q: this.invocationState.searchText,
            ...(this.invocationState.showEmptyGroups ? {} : { clean: '' }),
          });
          return newSearchParams.toString();
        },
        (newQueryStr) => {
          const location = window.location;
          const newUrl = `${location.protocol}//${location.host}${location.pathname}?${newQueryStr}`;
          window.history.replaceState({ path: newUrl }, '', newUrl);
        },
        { fireImmediately: true }
      )
    );
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
      Date.now() - this.appState.tabSelectionTime
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
        (display ? variants : []).map(
          (v, i, variants) =>
            [variants[i - 1], v, variants[i + 1]] as [TestVariant | undefined, TestVariant, TestVariant | undefined]
        ),
        ([_, v]) => `${v.testId} ${v.variantHash}`,
        ([prev, v, next]) => html`
          <milo-variant-entry
            .variant=${v}
            .prevTestId=${prev?.testId ?? ''}
            .prevVariant=${prev?.testId === v.testId ? prev : null}
            .expanded=${this.invocationState.testLoader?.testVariantCount === 1 || (prev === undefined && expandFirst)}
            .displayVariantId=${prev?.testId === v.testId || next?.testId === v.testId}
            .prerender=${true}
            .renderCallback=${this.variantRenderedCallback}
            .expandedCallback=${this.variantExpandedCallback}
          ></milo-variant-entry>
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

  private renderMain() {
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
      <milo-hotkey
        key="space,shift+space,up,down,pageup,pagedown"
        style="display: none;"
        .handler=${() => this.shadowRoot!.getElementById('test-result-view')!.focus()}
      ></milo-hotkey>
      <milo-lazy-list id="test-result-view" .growth=${300} tabindex="-1">${this.renderAllVariants()}</milo-lazy-list>
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
      <div id="main">${this.renderMain()}</div>
    `;
  }

  static styles = [
    commonStyle,
    colorClasses,
    css`
      :host {
        display: grid;
        grid-template-rows: auto 1fr;
        overflow-y: hidden;
      }

      #header {
        display: grid;
        grid-template-columns: auto auto auto 1fr auto;
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

      #main {
        display: flex;
        border-top: 1px solid var(--divider-color);
        overflow-y: hidden;
      }
      milo-lazy-list > * {
        padding-left: 10px;
      }
      #no-invocation {
        padding: 10px;
      }
      #test-result-view {
        flex: 1;
        overflow-y: auto;
        outline: none;
      }
      milo-variant-entry {
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
        font-size: 16px;
        font-weight: bold;
        padding: 5px 5px 5px 10px;
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
    `,
  ];
}
