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
import '@material/mwc-icon';
import { css, customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { autorun, computed, observable } from 'mobx';

import '../../components/test-entry';
import '../../components/test_filter';
import { TestFilter } from '../../components/test_filter';
import '../../components/test_nav_tree';
import { consumeContext } from '../../libs/context';
import { TestNode } from '../../models/test_node';
import { InvocationPageState } from './context';

/**
 * Display a list of test results.
 */
export class TestResultsTabElement extends MobxLitElement {
  @observable.ref pageState!: InvocationPageState;

  @observable.ref private leftPanelExpanded = false;
  @observable.ref private pageLength = 100;

  @computed
  private get rootName(): string {
    return this.pageState.testLoader.node.name;
  }
  private disposers: Array<() => void> = [];
  connectedCallback() {
    super.connectedCallback();
    this.pageState.selectedTabId = 'test-results';

    // Load more tests when there are more tests to be displayed but not loaded.
    this.disposers.push(autorun(() => {
      const state = this.pageState;
      if (state.selectedNode.fullyLoaded || state.testLoader.isLoading || this.pageLength <= state.selectedNode.allTests.length) {
        return;
      }
      state.testLoader.loadMore(this.pageLength - state.selectedNode.allTests.length);
    }));
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  protected render() {
    const state = this.pageState;
    return html`
      <div
        id="left-panel"
        style=${styleMap({display: this.leftPanelExpanded ? '' : 'none'})}
      >
        <tr-test-filter
          .onFilterChanged=${(filter: TestFilter) => {
            this.pageState.showExonerated = filter.showExonerated;
            this.pageState.showExpected = filter.showExpected;
          }}
        >
        </tr-test-filter>
        <tr-test-nav-tree
          .testLoader=${state.testLoader}
          .onSelectedNodeChanged=${(node: TestNode) => state.selectedNode = node}
        ></tr-test-nav-tree>
      </div>
      <div id="test-result-view">
        <div id="test-result-header">
          <div id="menu-button" @click=${() => this.leftPanelExpanded = !this.leftPanelExpanded}>
            <mwc-icon id="menu-icon">menu</mwc-icon>
          </div>
          <span id="root-name" title="common test ID prefix">${this.rootName}</span>
        </div>
        <div id="test-result-content">
          ${repeat(state.selectedNode.allTests.slice(0, this.pageLength), (t) => t.id, (t, i) => html`
          <tr-test-entry
            .test=${t}
            .prevTestId=${(state.selectedNode.allTests[i-1]?.id || '')}
            .expanded=${state.selectedNode.allTests.length === 1}
          ></tr-test-entry>
          `)}
          <div id="list-tail">
            <span>Showing ${Math.min(state.selectedNode.allTests.length, this.pageLength)}/${state.selectedNode.allTests.length}${state.selectedNode.fullyLoaded ? '' : '+'} tests.</span>
            <span
              id="load-more"
              style=${styleMap({'display': this.pageLength >= state.selectedNode.allTests.length && state.selectedNode.fullyLoaded ? 'none' : ''})}
              @click=${() => this.pageLength += 100}
            >
              Load More
            </span>
          </div>
        </div>
      </div>
    `;
  }

  static styles = css`
    :host {
      display: flex;
      border-top: 2px solid #DDDDDD;
      overflow-y: hidden;
    }

    #left-panel {
      display: grid;
      grid-template-rows: auto 1fr;
      border-right: 2px solid #DDDDDD;
      width: 400px;
      resize: horizontal;
      overflow-y: hidden;
    }
    tr-test-nav-tree {
      overflow: hidden;
    }
    #test-result-view {
      flex: 1;
      display: flex;
      flex-direction: column;
      overflow-y: hidden;
    }
    #test-result-header {
      width: 100%;
      height: 32px;
      background: #DDDDDD;
    }
    #menu-button {
      display: inline-table;
      height: 100%;
      cursor: pointer;
    }
    #menu-icon {
      display: table-cell;
      vertical-align: middle;
    }

    #test-result-content {
      overflow-y: auto;
    }
    #root-name {
      font-size: 16px;
      letter-spacing: 0.15px;
      vertical-align: middle;
      display: inline-table;
      height: 100%;
      margin-left: 5px;
    }

    #list-tail {
      margin: 5px;
    }
    #load-more {
      color: blue;
      cursor: pointer;
    }
  `;
}

customElement('tr-test-results-tab')(
  consumeContext('pageState')(
      TestResultsTabElement,
  ),
);
