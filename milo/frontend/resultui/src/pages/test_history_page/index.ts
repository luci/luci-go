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
import '@material/mwc-icon';
import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { DateTime } from 'luxon';
import { observable, reaction, when } from 'mobx';

import '../../components/overlay';
import '../../components/status_bar';
import '../../components/test_variants_table';
import '../../components/test_variants_table/tvt_config_widget';
import '../../components/hotkey';
import './date_axis';
import './duration_graph';
import './duration_legend';
import './graph_config';
import './filter_box';
import './status_graph';
import './variant_def_table';
import { MiloBaseElement } from '../../components/milo_base';
import { TestVariantsTableElement } from '../../components/test_variants_table';
import { provideTestVariantTableState } from '../../components/test_variants_table/context';
import { AppState, consumeAppState } from '../../context/app_state';
import { GraphType, provideTestHistoryPageState, TestHistoryPageState } from '../../context/test_history_page_state';
import { GA_ACTIONS, GA_CATEGORIES, generateRandomLabel, trackEvent } from '../../libs/analytics_utils';
import { consumer, provider } from '../../libs/context';
import { NOT_FOUND_URL } from '../../routes';
import commonStyle from '../../styles/common_style.css';

@customElement('milo-test-history-page')
@provider
@consumer
export class TestHistoryPageElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref @consumeAppState() appState!: AppState;
  @observable.ref @provideTestHistoryPageState() @provideTestVariantTableState() pageState!: TestHistoryPageState;

  @observable.ref private realm!: string;
  @observable.ref private testId!: string;
  private initialFilterText = '';

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const realm = location.params['realm'];
    const testId = location.params['test_id'];
    if (typeof testId !== 'string' || typeof realm !== 'string') {
      return cmd.redirect(NOT_FOUND_URL);
    }
    this.realm = realm;
    this.testId = testId;

    const searchParams = new URLSearchParams(location.search);
    if (searchParams.has('q')) {
      this.initialFilterText = searchParams.get('q')!;
    }

    return;
  }

  connectedCallback() {
    super.connectedCallback();

    const banner = html`This page is currently in beta and the loading time could be slow. See the
      <a href="https://crbug.com/1266759" target="_blank">tracking bug</a> for details.`;
    this.appState.addBanner(banner);
    this.addDisposer(() => this.appState.removeBanner(banner));

    // Set up TestHistoryPageState.
    this.addDisposer(
      reaction(
        () => [this.realm, this.testId, this.appState?.testHistoryService],
        () => {
          if (!this.realm || !this.testId || !this.appState.testHistoryService) {
            return;
          }

          const filterText = this.pageState ? this.pageState.filterText : this.initialFilterText;
          this.pageState?.dispose();

          this.pageState = new TestHistoryPageState(this.realm, this.testId, this.appState.testHistoryService);
          this.pageState.filterText = filterText;
          // Emulate @property() update.
          this.updated(new Map([['pageState', this.pageState]]));
        },
        {
          fireImmediately: true,
        }
      )
    );

    // Update the querystring when filters are updated.
    this.addDisposer(
      reaction(
        () => {
          const filterText = this.pageState ? this.pageState.filterText : this.initialFilterText;
          const newSearchParams = new URLSearchParams({
            ...(!filterText ? {} : { q: filterText }),
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

    // Track the time it takes to load all the test history in the past week.
    // The rendering time is not record. But it should be negligible.
    const oneWeekAgo = DateTime.now().minus({ weeks: 1 });
    this.addDisposer(
      when(
        () => {
          if (!this.pageState) {
            return false;
          }
          const loader = this.pageState.testHistoryLoader;
          if (loader.variants.length === 0) {
            return false;
          }
          return Boolean(loader.getEntries(loader.variants[0][0], oneWeekAgo));
        },
        () =>
          trackEvent(
            GA_CATEGORIES.HISTORY_PAGE,
            GA_ACTIONS.LOADING_TIME,
            generateRandomLabel(VISIT_ID + '_'),
            // TODO(weiweilin): this is only accurate when the test history page
            // is opened in a new tab. We should add a mechanism to consistently
            // record page selection time. (The first selected page should still
            // use TIME_ORIGIN as page selection time).
            Date.now() - TIME_ORIGIN
          )
      )
    );
  }

  private allVariantsWereExpanded = false;
  private toggleAllVariants(expand: boolean) {
    this.allVariantsWereExpanded = expand;
    this.shadowRoot!.querySelector<TestVariantsTableElement>('milo-test-variants-table')!.toggleAllVariants(expand);
  }
  private readonly toggleAllVariantsByHotkey = () => this.toggleAllVariants(!this.allVariantsWereExpanded);

  protected render() {
    return html`
      <table id="test-identifier">
        <tr>
          <td class="id-component-label">Realm</td>
          <td>${this.realm}</td>
        </tr>
        <tr>
          <td class="id-component-label">Test</td>
          <td>${this.testId}</td>
        </tr>
      </table>
      <milo-status-bar .components=${[{ color: 'var(--active-color)', weight: 1 }]}></milo-status-bar>
      ${this.renderBody()}${this.renderOverlay()}
    `;
  }

  private renderBody() {
    if (!this.pageState) {
      return html``;
    }

    return html`
      <milo-th-filter-box></milo-th-filter-box>
      <milo-th-graph-config></milo-th-graph-config>
      <div id="main">
        <milo-th-variant-def-table id="variant-def-table"></milo-th-variant-def-table>
        <milo-th-date-axis id="x-axis"></milo-th-date-axis>
        ${(() => {
          switch (this.pageState.graphType) {
            case GraphType.DURATION:
              return html`<milo-th-duration-graph id="graph"></milo-th-duration-graph>`;
            case GraphType.STATUS:
              return html`<milo-th-status-graph id="graph"></milo-th-status-graph>`;
          }
        })()}
        ${this.pageState.graphType === GraphType.DURATION
          ? html`<milo-th-duration-legend id="extra"></milo-th-duration-legend>`
          : ''}
      </div>
    `;
  }

  private renderOverlay() {
    if (!this.pageState) {
      return html``;
    }

    return html`
      <milo-overlay
        ?show=${this.pageState.selectedTvhEntries.length !== 0}
        @dismiss=${() => (this.pageState.selectedTvhEntries = [])}
      >
        <div id="tvt-container">
          <div id="tvt-header">
            <milo-tvt-config-widget id="tvt-config-widget"></milo-tvt-config-widget>
            <div><!-- GAP --></div>
            <milo-hotkey
              key="x"
              .handler=${this.toggleAllVariantsByHotkey}
              title="press x to expand/collapse all entries"
            >
              <mwc-button dense unelevated @click=${() => this.toggleAllVariants(true)}>Expand All</mwc-button>
              <mwc-button dense unelevated @click=${() => this.toggleAllVariants(false)}>Collapse All</mwc-button>
            </milo-hotkey>
            <milo-hotkey
              key="esc"
              .handler=${() => (this.pageState.selectedTvhEntries = [])}
              title="press esc to close the test variant details table"
            >
              <mwc-icon id="close-tvt" @click=${() => (this.pageState.selectedTvhEntries = [])}>close</mwc-icon>
            </milo-hotkey>
          </div>
          <milo-test-variants-table .hideTestName=${true} .showTimestamp=${true}></milo-test-variants-table>
        </div>
      </milo-overlay>
      <div
        id="free-scroll-padding"
        style=${styleMap({ display: this.pageState.selectedTvhEntries.length === 0 ? 'none' : '' })}
      ></div>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: grid;
        width: 100%;
        min-width: 800px;
        grid-template-rows: auto auto 1fr;
      }

      #test-identifier {
        width: 100%;
        background-color: var(--block-background-color);
        padding: 6px 16px;
        font-family: 'Google Sans', 'Helvetica Neue', sans-serif;
        font-size: 14px;
      }
      .id-component-label {
        color: var(--light-text-color);

        /* Shrink the first column */
        width: 0px;
      }

      milo-th-filter-box {
        width: calc(100% - 10px);
        margin: 5px;
      }

      #main {
        display: grid;
        margin: 0 5px;
        grid-template-columns: auto 1fr auto;
        grid-template-rows: auto 1fr;
        grid-template-areas:
          'v-table x-axis extra'
          'v-table graph extra';
      }

      #tvt-container {
        background-color: white;
        position: absolute;
        bottom: 0px;
        width: 100vw;
        height: 60vh;
        overflow-y: scroll;
      }

      #tvt-header {
        display: grid;
        grid-template-columns: auto 1fr auto auto;
        grid-gap: 5px;
        height: 30px;
        padding: 5px 10px 3px 10px;
        position: sticky;
        top: 0px;
        background: white;
        z-index: 3;
      }

      #tvt-config-widget {
        padding: 4px 5px 0px;
      }

      mwc-button {
        width: var(--expand-button-width);
      }

      #close-tvt {
        color: red;
        cursor: pointer;
        padding-top: 2px;
      }

      milo-test-variants-table {
        --tvt-top-offset: 38px;
      }

      #free-scroll-padding {
        width: 100%;
        height: 60vh;
      }

      #variant-def-table {
        grid-area: v-table;
      }

      #x-axis {
        grid-area: x-axis;
      }

      #graph {
        grid-area: graph;
      }

      #extra {
        grid-area: extra;
      }
    `,
  ];
}
