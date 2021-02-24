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
import '@material/mwc-button';
import '@material/mwc-icon';
import { BeforeEnterObserver } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction } from 'mobx';

import '../components/dot_spinner';
import '../components/hotkey';
import '../components/test_search_filter';
import '../components/variant_entry';
import { VariantEntryElement } from '../components/variant_entry';
import { AppState, consumeAppState } from '../context/app_state';
import { consumeInvocationState, InvocationState } from '../context/invocation_state';
import { consumeConfigsStore, UserConfigsStore } from '../context/user_configs';
import { GA_ACTIONS, GA_CATEGORIES, generateRandomLabel, trackEvent } from '../libs/analytics_utils';
import { VARIANT_STATUS_CLASS_MAP, VARIANT_STATUS_DISPLAY_MAP, VARIANT_STATUS_DISPLAY_MAP_TITLE_CASE } from '../libs/constants';
import { TestVariant, TestVariantStatus } from '../services/resultdb';

/**
 * Display a list of test results.
 */
export class TestResultsTabElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref invocationState!: InvocationState;
  private enterTimestamp = 0;
  private sentLoadingTimeToGA = false;

  private disposers: Array<() => void> = [];
  private async loadNextPage() {
    try {
      await this.invocationState.testLoader?.loadNextPage();
    } catch (e) {
      this.dispatchEvent(new ErrorEvent('error', {
        error: e,
        message: e.toString(),
        composed: true,
        bubbles: true,
      }));
    }
  }

  /**
   * Loads pages until we receive some variants with the given variant status.
   *
   * Will always load at least one page.
   */
  private async loadPagesUntilStatus(status: TestVariantStatus) {
    try {
      await this.invocationState.testLoader?.loadPagesUntilStatus(status);
    } catch (e) {
      this.dispatchEvent(new ErrorEvent('error', {
        error: e,
        message: e.toString(),
        composed: true,
        bubbles: true,
      }));
    }
  }

  onBeforeEnter() {
    this.enterTimestamp = Date.now();
  }

  @computed
  private get totalDisplayedVariantCount() {
    let count = 0;
    if (this.invocationState.showUnexpectedVariants) {
      count += this.invocationState.filteredUnexpectedVariants.length;
    }
    if (this.invocationState.showFlakyVariants) {
      count += this.invocationState.filteredFlakyVariants.length;
    }
    if (this.invocationState.showExoneratedVariants) {
      count += this.invocationState.filteredExoneratedVariants.length;
    }
    if (this.invocationState.showExpectedVariants) {
      count += this.invocationState.filteredExpectedVariants.length;
    }

    return count;
  }

  @observable.ref private allVariantsWereExpanded = false;
  private toggleAllVariants(expand: boolean) {
    this.allVariantsWereExpanded = expand;
    this.shadowRoot!.querySelectorAll<VariantEntryElement>('milo-variant-entry')
      .forEach((e) => e.expanded = expand);
  }
  private readonly toggleAllVariantsByHotkey = () => this.toggleAllVariants(!this.allVariantsWereExpanded);

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'test-results';
    trackEvent(GA_CATEGORIES.TEST_RESULTS_TAB, GA_ACTIONS.TAB_VISITED);

    // If first page of test results has already been loaded when connected
    // (happens when users switch tabs), we don't want to track the loading
    // time (only a few ms in this case)
    if (this.invocationState.testLoader?.firstPageLoaded) {
      this.sentLoadingTimeToGA = true;
    }

    // When a new test loader is received, load the first page and reset the
    // selected node.
    this.disposers.push(reaction(
      () => this.invocationState.testLoader,
      (testLoader) => {
        if (!testLoader) {
          return;
        }
        // The previous instance of the test results tab could've triggered
        // the loading operation already. In that case we don't want to load
        // more test results.
        if (!testLoader.firstRequestSent) {
          this.loadNextPage();
        }
      },
      {fireImmediately: true},
    ));

    // Update filters to match the querystring without saving them.
    const searchParams = new URLSearchParams(window.location.search);
    if (searchParams.has('q')) {
      this.invocationState.searchText = searchParams.get('q')!;
    }
    // Update the querystring when filters are updated.
    this.disposers.push(reaction(
      () => {
        const newSearchParams = new URLSearchParams({
          q: this.invocationState.searchText,
        });
        return newSearchParams.toString();
      },
      (newQueryStr) => {
        const newUrl = `${window.location.protocol}//${window.location.host}${window.location.pathname}?${newQueryStr}`;
        window.history.replaceState({path: newUrl}, '', newUrl);
      },
      {fireImmediately: true},
    ));
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  private renderAllVariants() {
    return html`
      ${this.renderIntegrationHint()}
      ${this.renderVariants(
        TestVariantStatus.UNEXPECTED,
        this.invocationState.filteredUnexpectedVariants,
        this.invocationState.showUnexpectedVariants,
        (display) => this.invocationState.showUnexpectedVariants = display,
        this.invocationState.testLoader?.loadedAllUnexpectedVariants || false,
        true,
      )}
      ${this.renderVariants(
        TestVariantStatus.UNEXPECTEDLY_SKIPPED,
        this.invocationState.filteredUnexpectedlySkippedVariants,
        this.invocationState.showUnexpectedlySkippedVariants,
        (display) => this.invocationState.showUnexpectedlySkippedVariants = display,
        this.invocationState.testLoader?.loadedAllUnexpectedlySkippedVariants || false,
      )}
      ${this.renderVariants(
        TestVariantStatus.FLAKY,
        this.invocationState.filteredFlakyVariants,
        this.invocationState.showFlakyVariants,
        (display) => this.invocationState.showFlakyVariants = display,
        this.invocationState.testLoader?.loadedAllFlakyVariants || false,
      )}
      ${this.renderVariants(
        TestVariantStatus.EXONERATED,
        this.invocationState.filteredExoneratedVariants,
        this.invocationState.showExoneratedVariants,
        (display) => this.invocationState.showExoneratedVariants = display,
        this.invocationState.testLoader?.loadedAllExoneratedVariants || false,
      )}
      ${this.renderVariants(
        TestVariantStatus.EXPECTED,
        this.invocationState.filteredExpectedVariants,
        this.invocationState.showExpectedVariants,
        (display) => this.invocationState.showExpectedVariants = display,
        this.invocationState.testLoader?.loadedAllExpectedVariants || false,
      )}
    `;
  }

  private renderIntegrationHint() {
    return this.configsStore.userConfigs.hints.showTestResultsHint ? html `
      <div class="list-entry">
        <p>
        Don't see results of your test framework here?
        This might be because they are not integrated with ResultDB yet.
        Please ask <a href="mailto: luci-eng@google.com" target="_blank">luci-eng@</a> for help.
        </p>
        Known issues:
        <ul id="known-issues">
          <li>Test result tab is currently slow: <a href="https://crbug.com/1114935">crbug.com/1114935</a>.</li>
        </ul>
        <span
          id="hide-hint"
          @click=${() => {
            this.configsStore.userConfigs.hints.showTestResultsHint = false;
            this.configsStore.save();
          }}
        >Don't show again</span>
      </div>
      <hr class="divider">
    `: html ``;
  }

  private sendLoadingTimeToGA() {
    if (this.sentLoadingTimeToGA) {
      return;
    }
    this.sentLoadingTimeToGA = true;
    const prefix = 'testresults_' + this.invocationState.invocationId;
    trackEvent(
      GA_CATEGORIES.TEST_RESULTS_TAB,
      GA_ACTIONS.LOADING_TIME,
      generateRandomLabel(prefix),
      Date.now() - this.enterTimestamp,
    );
  }

  private variantRenderedCallback = () => {
    this.sendLoadingTimeToGA();
  }

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
    expandFirst = false,
  ) {
    const variantCountLabel = `${variants.length}${fullyLoaded ? '' : '+'}`;
    return html`
      <div class="section-header">
        ${VARIANT_STATUS_DISPLAY_MAP_TITLE_CASE[status]} (${variantCountLabel})
        <span
          class="active-text"
          @click=${() => toggleDisplay(!display)}
        >[${display ? 'hide' : 'show'}]</span>
        <span
          class="active-text"
          style=${styleMap({'display': fullyLoaded ? 'none' : ''})}
        >${this.renderLoadMoreForSection(status)}</span>
      </div>
      ${repeat(
        (display ? variants : []).map((v, i, variants) => [variants[i-1], v, variants[i+1]] as [TestVariant | undefined, TestVariant, TestVariant | undefined]),
        ([_, v]) => `${v.testId} ${v.variantHash}`,
        ([prev, v, next]) => html`
        <milo-variant-entry
          .variant=${v}
          .prevTestId=${prev?.testId ?? ''}
          .prevVariant=${prev?.testId === v.testId ? prev : null}
          .expanded=${this.totalDisplayedVariantCount === 1 || (prev === undefined && expandFirst)}
          .displayVariantId=${prev?.testId === v.testId || next?.testId === v.testId}
          .prerender=${true}
          .renderCallback=${this.variantRenderedCallback}
        ></milo-variant-entry>
      `)}
      <div
        class="list-entry"
        style=${styleMap({'display': variants.length === 0 && fullyLoaded ? '' : 'none'})}
      >
        No
        <span class=${VARIANT_STATUS_CLASS_MAP[status]}>${VARIANT_STATUS_DISPLAY_MAP[status]}</span>
        test results.
      </div>
      <div
        class="list-entry"
        style=${styleMap({'display': !display && variants.length !== 0 ? '' : 'none'})}
      >
        ${variants.length} hidden
        <span class=${VARIANT_STATUS_CLASS_MAP[status]}>${VARIANT_STATUS_DISPLAY_MAP[status]}</span>
        test results.
      </div>
      <hr class="divider">
    `;
  }

  private renderLoadMoreForSection(status: TestVariantStatus) {
    const state = this.invocationState;
    return html`
      <span
        style=${styleMap({'display': state.testLoader?.isLoading ?? true ? 'none' : ''})}
        @click=${() => this.loadPagesUntilStatus(status)}
      >
        [load more]
      </span>
      <span style=${styleMap({'display': state.testLoader?.isLoading ?? true ? '' : 'none'})}>
        loading <milo-dot-spinner></milo-dot-spinner>
      </span>
    `;
  }

  private renderMain() {
    const state = this.invocationState;

    if (state.invocationId === '') {
      return html`
        <div id="no-invocation">
          No associated invocation.<br>
          You need to integrate with ResultDB to see the test results.<br>
          See <a href="http://go/resultdb" target="_blank">go/resultdb</a>
          or ask <a href="mailto: luci-eng@google.com" target="_blank">luci-eng@</a> for help.
        </div>
      `;
    }

    return html`
      <milo-hotkey
        key="space,shift+space,up,down,pageup,pagedown"
        style="display: none;"
        .handler=${() => this.shadowRoot!.getElementById('test-result-view')!.focus()}
      ></milo-hotkey>
      <milo-lazy-list
        id="test-result-view"
        .growth=${300} tabindex="-1"
      >${this.renderAllVariants()}</milo-lazy-list>
    `;
  }

  protected render() {
    return html`
      <div id="header">
        <div id="search-label">Search:</div>
        <milo-test-search-filter></milo-test-search-filter>
        <milo-hotkey key="x" .handler=${this.toggleAllVariantsByHotkey} title="press x to expand/collapse all entries">
          <mwc-button
            dense unelevated
            @click=${() => this.toggleAllVariants(true)}
          >Expand All</mwc-button>
          <mwc-button
            dense unelevated
            @click=${() => this.toggleAllVariants(false)}
          >Collapse All</mwc-button>
        </milo-hotkey>
      </div>
      <div id="main">${this.renderMain()}</div>
    `;
  }

  static styles = css`
    :host {
      display: grid;
      grid-template-rows: auto 1fr;
      overflow-y: hidden;
    }

    #header {
      display: grid;
      grid-template-columns: auto 1fr auto;
      grid-gap: 5px;
      height: 30px;
      padding: 5px 10px 3px 10px;
    }
    #search-label {
      margin: auto;
    }
    milo-test-search-filter {
      max-width: 800px;
    }
    mwc-button {
      margin-top: 1px;
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
      padding-left: 10px;
    }
    #no-invocation {
      padding: 10px;
    }
    #test-result-view {
      flex: 1;
      overflow-y: auto;
      margin-top: 5px;
      outline: none;
    }
    milo-variant-entry {
      margin-bottom: 2px;
    }

    .section-header {
      font-size: 16px;
      font-weight: bold;
      padding: 5px;
      position: sticky;
      background-color: white;
      top: 0px;
    }

    .divider {
      border: none;
      border-top: 1px solid var(--divider-color);
    }

    milo-test-nav-tree {
      overflow: hidden;
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

    .list-entry {
      margin: 5px;
    }
    .active-text {
      color: var(--active-text-color);
      cursor: pointer;
      font-size: 14px;
      font-weight: normal;
    }
    .inline-icon {
      color: var(--default-text-color);
      --mdc-icon-size: 1.2em;
      vertical-align: bottom;
    }
    #hide-hint {
      color: var(--active-text-color);
      cursor: pointer;
    }
    #known-issues{
      margin-top: 3px;
      margin-bottom: 3px;
    }
  `;
}

customElement('milo-test-results-tab')(
  consumeInvocationState(
    consumeConfigsStore(
      consumeAppState(TestResultsTabElement),
    ),
  ),
);
