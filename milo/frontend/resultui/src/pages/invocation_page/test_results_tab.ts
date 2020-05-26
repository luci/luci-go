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
import { css, customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { observable, reaction } from 'mobx';

import '../../components/left_panel';
import '../../components/test-entry';
import '../../components/test_filter';
import { TestFilter } from '../../components/test_filter';
import '../../components/test_nav_tree';
import { consumeContext } from '../../libs/context';
import * as iter from '../../libs/iter_utils';
import { TestNode } from '../../models/test_node';
import { InvocationPageState } from './context';

/**
 * Display a list of test results.
 */
export class TestResultsTabElement extends MobxLitElement {
  @observable.ref pageState!: InvocationPageState;

  private disposers: Array<() => void> = [];
  connectedCallback() {
    super.connectedCallback();
    this.pageState.selectedTabId = 'test-results';

    // Load the first page when a new test loader is received.
    this.disposers.push(reaction(
      () => this.pageState.testLoader,
      (testLoader) => testLoader.loadNextPage(),
    ));
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
      <tr-test-filter
        .onFilterChanged=${(filter: TestFilter) => {
          this.pageState.showExonerated = filter.showExonerated;
          this.pageState.showExpected = filter.showExpected;
        }}
      >
      </tr-test-filter>
      <div id="main">
        <tr-left-panel>
          <tr-test-nav-tree
            .testLoader=${state.testLoader}
            .onSelectedNodeChanged=${(node: TestNode) => state.selectedNode = node}
          ></tr-test-nav-tree>
        </tr-left-panel>
        <div id="test-result-view">
          ${repeat(iter.withPrev(state.selectedNode.tests()), ([t]) => t.id, ([t, prev]) => html`
          <tr-test-entry
            .test=${t}
            .prevTestId=${(prev?.id || '')}
            .expanded=${state.selectedNode.testCount === 1}
          ></tr-test-entry>
          `)}
          <div id="list-tail">
            <span>Showing ${state.selectedNode.testCount} tests.</span>
            <span
              id="load"
              style=${styleMap({'display': state.testLoader.done ? 'none' : ''})}
            >
              <span
                id="load-more"
                style=${styleMap({'display': state.testLoader.isLoading ? 'none' : ''})}
                @click=${() => state.testLoader.loadNextPage()}
              >
                Load More
              </span>
              <span
                style=${styleMap({'display': state.testLoader.isLoading ? '' : 'none'})}
              >
                Loading <tr-dot-spinner></tr-dot-spinner>
              </span>
            </span>
          </div>
        </div>
      </div>
    `;
  }

  static styles = css`
    :host {
      display: grid;
      grid-template-rows: auto 1fr;
      overflow-y: hidden;
    }

    #main {
      display: flex;
      border-top: 1px solid #DDDDDD;
      overflow-y: hidden;
    }
    #test-result-view {
      flex: 1;
      display: flex;
      flex-direction: column;
      overflow-y: auto;
      padding-top: 5px;
    }

    tr-test-nav-tree {
      overflow: hidden;
    }

    #list-tail {
      margin: 5px;
    }
    #load {
      color: blue;
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
