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
import { computed, makeObservable, observable, reaction } from 'mobx';

import '../../components/dot_spinner';
import '../../components/overlay';
import '../../components/status_bar';
import './test_history_details_table';
import './test_history_details_table/config_widget';
import '../../components/hotkey';
import './date_axis';
import './duration_graph';
import './duration_legend';
import './graph_config';
import './filter_box';
import './status_graph';
import './variant_def_table';
import { MiloBaseElement } from '../../components/milo_base';
import { consumer, provider } from '../../libs/context';
import { NOT_FOUND_URL } from '../../routes';
import { consumeStore, StoreInstance } from '../../store';
import { GraphType } from '../../store/test_history_page';
import commonStyle from '../../styles/common_style.css';
import { TestHistoryDetailsTableElement } from './test_history_details_table';

const LOADING_VARIANT_INFO_TOOLTIP =
  'It may take several clicks to find any new variant. ' +
  'If you know what your are looking for, please apply a filter instead. ' +
  'This will be improved the in future.';

@customElement('milo-test-history-page')
@provider
@consumer
export class TestHistoryPageElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref @consumeStore() store!: StoreInstance;
  @computed get pageState() {
    return this.store.testHistoryPage;
  }

  @observable.ref private realm!: string;
  @observable.ref private testId!: string;
  private initialFilterText = '';

  constructor() {
    super();
    makeObservable(this);
  }

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

    // Set up TestHistoryPageState.
    this.addDisposer(
      reaction(
        () => [this.realm, this.testId, this.store?.services.testHistory],
        () => {
          if (!this.realm || !this.testId) {
            return;
          }

          this.pageState.setParams(this.realm, this.testId);
          if (this.initialFilterText) {
            this.pageState.setFilterText(this.initialFilterText);
            this.initialFilterText = '';
          }
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
          const newSearchParams = new URLSearchParams({
            ...(!this.pageState.filterText ? {} : { q: this.pageState.filterText }),
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

    this.addDisposer(
      reaction(
        () => this.pageState.entriesLoader,
        (loader) => loader?.loadFirstPage(),
        { fireImmediately: true }
      )
    );

    this.addDisposer(
      reaction(
        () => this.pageState.variantsLoader,
        (loader) => loader?.loadFirstPage(),
        { fireImmediately: true }
      )
    );
  }

  private allVariantsWereExpanded = false;
  private toggleAllVariants(expand: boolean) {
    this.allVariantsWereExpanded = expand;
    this.shadowRoot!.querySelector<TestHistoryDetailsTableElement>(
      'milo-test-history-details-table'
    )!.toggleAllVariants(expand);
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
      ${this.renderBody()}${this.renderVariantCount()}${this.renderOverlay()}
    `;
  }

  private renderBody() {
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

  private renderVariantCount() {
    return html`
      <div id="variant-count">
        Showing
        <i>${this.pageState.filteredVariants.length}</i>
        variant${this.pageState.filteredVariants.length === 1 ? '' : 's'} that
        <i>match${this.pageState.filteredVariants.length === 1 ? '' : 'es'} the filter</i>.
        <span>
          ${
            !(this.pageState.variantsLoader?.isLoading ?? true) && !this.pageState.variantsLoader?.loadedAll
              ? html`
                  <span class="active-text" @click=${() => this.pageState.variantsLoader?.loadNextPage()}>
                    [load more]
                  </span>
                  <mwc-icon class="inline-icon" title=${LOADING_VARIANT_INFO_TOOLTIP}>info</mwc-icon>
                `
              : ''
          }
          ${
            this.pageState.variantsLoader?.isLoading ?? true
              ? html` <span class="active-text">loading <milo-dot-spinner></milo-dot-spinner></span>`
              : ''
          }
      </div>
    `;
  }

  private renderOverlay() {
    return html`
      <milo-overlay
        .show=${Boolean(this.pageState.selectedGroup)}
        @dismiss=${() => this.pageState.setSelectedGroup(null)}
      >
        <div id="thdt-container">
          <div id="thdt-header">
            <milo-thdt-config-widget id="thdt-config-widget"></milo-thdt-config-widget>
            <div><!-- GAP --></div>
            <milo-hotkey
              .key=${'x'}
              .handler=${this.toggleAllVariantsByHotkey}
              title="press x to expand/collapse all entries"
            >
              <mwc-button dense unelevated @click=${() => this.toggleAllVariants(true)}>Expand All</mwc-button>
              <mwc-button dense unelevated @click=${() => this.toggleAllVariants(false)}>Collapse All</mwc-button>
            </milo-hotkey>
            <milo-hotkey
              .key=${'esc'}
              .handler=${() => this.pageState.setSelectedGroup(null)}
              title="press esc to close the test variant details table"
            >
              <mwc-icon id="close-tvt" @click=${() => this.pageState.setSelectedGroup(null)}>close</mwc-icon>
            </milo-hotkey>
          </div>
          <milo-test-history-details-table></milo-test-history-details-table>
        </div>
      </milo-overlay>
      <div id="free-scroll-padding" style=${styleMap({ display: this.pageState.selectedGroup ? 'none' : '' })}></div>
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

      #thdt-container {
        background-color: white;
        position: absolute;
        bottom: 0px;
        width: 100vw;
        height: 60vh;
        overflow-y: scroll;
      }

      #thdt-header {
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

      #thdt-config-widget {
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

      milo-test-history-details-table {
        --thdt-top-offset: 38px;
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

      #variant-count {
        padding: 5px;
      }

      .inline-icon {
        --mdc-icon-size: 1.2em;
        vertical-align: text-top;
      }
    `,
  ];
}
