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
import { autorun, observable } from 'mobx';

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

  @observable.ref private pageLength = 100;

  private disposers: Array<() => void> = [];
  connectedCallback() {
    super.connectedCallback();
    this.pageState.selectedTabId = 'test-results';

    // Load more tests when there are more tests to be displayed but not loaded.
    this.disposers.push(autorun(() => {
      const state = this.pageState;
      if (state.testLoader.done || state.testLoader.isLoading || this.pageLength <= state.selectedNode.testCount) {
        return;
      }
      state.testLoader.loadMore(this.pageLength - state.selectedNode.testCount);
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
          ${repeat(iter.withPrev(iter.take(state.selectedNode.tests(), this.pageLength)), ([t]) => t.id, ([t, prev]) => html`
          <tr-test-entry
            .test=${t}
            .prevTestId=${(prev?.id || '')}
            .expanded=${state.selectedNode.testCount === 1}
          ></tr-test-entry>
          `)}
          <div id="list-tail">
            <span>Showing ${Math.min(state.selectedNode.testCount, this.pageLength)}/${state.selectedNode.testCount}${state.testLoader.done ? '' : '+'} tests.</span>
            <span
              id="load-more"
              style=${styleMap({'display': this.pageLength >= state.selectedNode.testCount && state.testLoader.done ? 'none' : ''})}
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
