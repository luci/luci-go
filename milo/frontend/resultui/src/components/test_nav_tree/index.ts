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
import { css, customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import { observable, reaction } from 'mobx';

import { TestLoader } from '../../models/test_loader';
import { TestNode } from '../../models/test_node';
import '../dot_spinner';
import { provideTreeState, TestNavTreeState } from './context';
import './test_nav_node';


/**
 * Display all test IDs in a folder-like tree structure.
 * Segments ending with /\W/ in test IDs are treated as "folders".
 * When a node has no sibling, it's collapsed into its parent.
 */
export class TestNavTreeElement extends MobxLitElement {
  @observable.ref testLoader!: TestLoader;
  onSelectedNodeChanged: (node: TestNode) => void = () => {};

  treeState = new TestNavTreeState();

  private disposer: () => void = () => {};
  connectedCallback() {
    super.connectedCallback();
    // Notify the listener when the selected node is changed.
    this.disposer = reaction(
      // When no child node is selected, select the root node.
      () => this.treeState.selectedChildNode || this.testLoader.node,
      (node) => this.onSelectedNodeChanged(node),
      {fireImmediately: true},
    );
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    this.disposer();
  }

  private async loadNextPage() {
    try {
      await this.testLoader.loadNextPage();
    } catch (e) {
      this.dispatchEvent(new ErrorEvent('error', {
        error: e,
        message: e.toString(),
        composed: true,
        bubbles: true,
      }));
    }
  }

  protected render() {
    const root = this.testLoader?.node;

    return html`
      <div id="container">
        <div id="header" title=${root.name}>${root.name}</div>
        <div id="body">
          ${repeat(root.children, (node) => node.name, (node) => html`
          <tr-test-nav-node
            .depth=${0}
            .node=${node}
          >
          </tr-test-nav-node>
          `)}
          <mwc-button
            id="load-more-btn"
            unelevated dense
            @click=${this.loadNextPage}
            ?disabled=${this.testLoader.done || this.testLoader.isLoading}
          >
            ${this.testLoader.isLoading ?
              html`Loading <tr-dot-spinner></tr-dot-spinner>` :
              this.testLoader.done ? 'All Tests are Loaded' : 'Load the next 100 Tests'
            }
          </mwc-button>
        </div>
      </div>
    `;
  }

  static styles = css`
    #container {
      height: 100%;
      display: grid;
      grid-template-rows: auto 1fr;
    }

    #header {
      background-color: #DDDDDD;
      padding: 2px 5px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }

    #body {
      overflow-y: auto;
    }

    #load-more-btn {
      --mdc-theme-primary: rgb(0, 123, 255);
      width: calc(100% - 10px);
      margin: 0 5px;
    }
  `;
}

customElement('tr-test-nav-tree')(
  provideTreeState(
    TestNavTreeElement,
  ),
);
