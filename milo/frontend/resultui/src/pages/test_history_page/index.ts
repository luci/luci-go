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

import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, customElement, html, property } from 'lit-element';
import { comparer, computed, observable, reaction } from 'mobx';

import './graph_config';
import '../../components/status_bar';
import './status_graph';
import { MiloBaseElement } from '../../components/milo_base';
import { AppState, consumeAppState } from '../../context/app_state';
import { provideTestHistoryPageState, TestHistoryPageState } from '../../context/test_history_page_state';
import { consumer, provider } from '../../libs/context';
import { NOT_FOUND_URL } from '../../routes';
import { getCriticalVariantKeys } from '../../services/resultdb';
import commonStyle from '../../styles/common_style.css';
import { CELL_SIZE, X_AXIS_HEIGHT } from './constants';

@customElement('milo-test-history-page')
@provider
@consumer
export class TestHistoryPageElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref @consumeAppState() appState!: AppState;
  @property() @provideTestHistoryPageState() pageState!: TestHistoryPageState;

  @observable.ref private realm!: string;
  @observable.ref private testId!: string;

  @computed private get variants() {
    return this.pageState.testHistoryLoader?.variants || [];
  }

  @computed({ equals: comparer.shallow }) private get criticalVariantKeys() {
    return getCriticalVariantKeys(this.variants.map(([_, v]) => v));
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const realm = location.params['realm'];
    const testId = location.params['test_id'];
    if (typeof testId !== 'string' || typeof realm !== 'string') {
      return cmd.redirect(NOT_FOUND_URL);
    }
    this.realm = realm;
    this.testId = testId;

    return;
  }

  connectedCallback() {
    super.connectedCallback();

    // Set up TestHistoryPageState.
    this.addDisposer(
      reaction(
        () => [this.realm, this.testId, this.appState?.testHistoryService],
        () => {
          if (!this.realm || !this.testId || !this.appState.testHistoryService) {
            return;
          }

          this.pageState?.dispose();
          this.pageState = new TestHistoryPageState(this.realm, this.testId, this.appState.testHistoryService);
        },
        {
          fireImmediately: true,
        }
      )
    );
  }

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
      ${this.renderBody()}
    `;
  }

  private renderBody() {
    if (!this.pageState) {
      return html``;
    }

    return html`
      <milo-th-graph-config></milo-th-graph-config>
      <div id="main">
        <table id="variant-def-table">
          <tr>
            ${this.criticalVariantKeys.map((k) => html`<th>${k}</th>`)}
          </tr>
          ${this.variants.map(
            ([_, v]) => html`
              <tr>
                ${this.criticalVariantKeys.map((k) => html`<td>${v.def[k] || ''}</td>`)}
              </tr>
            `
          )}
        </table>
        <milo-th-status-graph></milo-th-status-graph>
      </div>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: grid;
        width: 100%;
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

      #main {
        display: grid;
        grid-template-columns: auto 1fr;
      }

      #variant-def-table {
        padding-top: ${X_AXIS_HEIGHT - CELL_SIZE}px;
        border-spacing: 0;
      }
      #variant-def-table tr:first-child {
      }
      #variant-def-table tr {
        height: ${CELL_SIZE}px;
      }
      #variant-def-table tr:nth-child(even) {
        background-color: var(--block-background-color);
      }
      #variant-def-table * {
        padding: 0;
      }
      #variant-def-table td {
        text-align: center;
        padding: 0 2px;
      }
    `,
  ];
}
