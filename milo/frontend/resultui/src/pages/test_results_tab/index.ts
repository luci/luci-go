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

import '@material/mwc-button';
import { css, customElement, html } from 'lit-element';
import { makeObservable, observable, reaction } from 'mobx';

import '../../components/dot_spinner';
import '../../components/hotkey';
import './test_variants_table';
import './test_variants_table/config_widget';
import './search_box';
import { MiloBaseElement } from '../../components/milo_base';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state';
import { GA_ACTIONS, GA_CATEGORIES, trackEvent } from '../../libs/analytics_utils';
import { consumer } from '../../libs/context';
import { errorHandler, forwardWithoutMsg, reportRenderError } from '../../libs/error_handler';
import { consumeStore, StoreInstance } from '../../store';
import commonStyle from '../../styles/common_style.css';
import { TestVariantsTableElement } from './test_variants_table';

/**
 * Display test results.
 */
@customElement('milo-test-results-tab')
@errorHandler(forwardWithoutMsg)
@consumer
export class TestResultsTabElement extends MiloBaseElement {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  @observable.ref
  @consumeInvocationState()
  invocationState!: InvocationState;

  private allVariantsWereExpanded = false;
  private toggleAllVariants(expand: boolean) {
    this.allVariantsWereExpanded = expand;
    this.shadowRoot!.querySelector<TestVariantsTableElement>('milo-test-variants-table')!.toggleAllVariants(expand);
  }
  private readonly toggleAllVariantsByHotkey = () => this.toggleAllVariants(!this.allVariantsWereExpanded);

  constructor() {
    super();
    makeObservable(this);
  }

  connectedCallback() {
    super.connectedCallback();
    this.store.setSelectedTabId('test-results');
    trackEvent(GA_CATEGORIES.TEST_RESULTS_TAB, GA_ACTIONS.TAB_VISITED, window.location.href);

    // Update filters to match the querystring without saving them.
    const searchParams = new URLSearchParams(window.location.search);
    if (searchParams.has('q')) {
      this.invocationState.searchText = searchParams.get('q')!;
    }
    if (searchParams.has('cols')) {
      const cols = searchParams.get('cols')!;
      this.invocationState.setColumnKeys(cols.split(',').filter((col) => col !== ''));
    }
    if (searchParams.has('sortby')) {
      const sortingKeys = searchParams.get('sortby')!;
      this.invocationState.setSortingKeys(sortingKeys.split(',').filter((col) => col !== ''));
    }
    if (searchParams.has('groupby')) {
      const groupingKeys = searchParams.get('groupby')!;
      this.invocationState.setGroupingKeys(groupingKeys.split(',').filter((key) => key !== ''));
    }

    // Update the querystring when filters are updated.
    this.addDisposer(
      reaction(
        () => {
          const displayedCols = this.invocationState.columnKeys.join(',');
          const defaultCols = this.invocationState.defaultColumnKeys.join(',');
          const sortingKeys = this.invocationState.sortingKeys.join(',');
          const defaultSortingKeys = this.invocationState.defaultSortingKeys.join(',');
          const groupingKeys = this.invocationState.groupingKeys.join(',');
          const defaultGroupingKeys = this.invocationState.defaultGroupingKeys.join(',');

          const newSearchParams = new URLSearchParams({
            ...(!this.invocationState.searchText ? {} : { q: this.invocationState.searchText }),
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
          window.history.replaceState(null, '', newUrl);
        },
        { fireImmediately: true }
      )
    );
  }

  private renderBody() {
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
      ${this.invocationState.warning
        ? html`<div id="test-results-tab-warning">${this.invocationState.warning}</div>`
        : ''}
      <milo-test-variants-table></milo-test-variants-table>
    `;
  }

  protected render = reportRenderError(this, () => {
    return html`
      <div id="header">
        <milo-tvt-config-widget class="filters-container"></milo-tvt-config-widget>
        <div class="filters-container-delimiter"></div>
        <milo-trt-search-box></milo-trt-search-box>
        <milo-hotkey .key="x" .handler=${this.toggleAllVariantsByHotkey} title="press x to expand/collapse all entries">
          <mwc-button dense unelevated @click=${() => this.toggleAllVariants(true)}>Expand All</mwc-button>
          <mwc-button dense unelevated @click=${() => this.toggleAllVariants(false)}>Collapse All</mwc-button>
        </milo-hotkey>
      </div>
      ${this.renderBody()}
    `;
  });

  static styles = [
    commonStyle,
    css`
      :host {
        /* Required to make 'position: sticky' work in Safari. */
        display: block;
      }

      #header {
        display: grid;
        grid-template-columns: auto auto 1fr auto;
        grid-gap: 5px;
        height: 30px;
        padding: 5px 10px 3px 10px;
        position: sticky;
        top: 0px;
        background: white;
        z-index: 3;
      }
      milo-trt-search-box {
        max-width: 1000px;
      }
      mwc-button {
        margin-top: 1px;
        width: var(--expand-button-width);
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

      #test-results-tab-warning {
        background-color: var(--warning-color);
        border-bottom: 1px solid var(--divider-color);
        height: 24px;
        line-height: 24px;
        text-align: center;
      }

      milo-test-variants-table {
        --tvt-top-offset: 38px;
      }
    `,
  ];
}
