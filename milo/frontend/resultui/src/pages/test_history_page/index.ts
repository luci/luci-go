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
import { observable, reaction } from 'mobx';

import '../../components/status_bar';
import './date_axis';
import './duration_graph';
import './duration_scale';
import './graph_config';
import './status_graph';
import './variant_def_table';
import { MiloBaseElement } from '../../components/milo_base';
import { AppState, consumeAppState } from '../../context/app_state';
import { GraphType, provideTestHistoryPageState, TestHistoryPageState } from '../../context/test_history_page_state';
import { consumer, provider } from '../../libs/context';
import { NOT_FOUND_URL } from '../../routes';
import commonStyle from '../../styles/common_style.css';

@customElement('milo-test-history-page')
@provider
@consumer
export class TestHistoryPageElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref @consumeAppState() appState!: AppState;
  @property() @provideTestHistoryPageState() pageState!: TestHistoryPageState;

  @observable.ref private realm!: string;
  @observable.ref private testId!: string;

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
          ? html`<milo-th-duration-scale id="extra"></milo-th-duration-scale>`
          : ''}
      </div>
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

      #main {
        display: grid;
        margin: 0 5px;
        grid-template-columns: auto 1fr auto;
        grid-template-rows: auto 1fr;
        grid-template-areas:
          'v-table x-axis extra'
          'v-table graph extra';
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
