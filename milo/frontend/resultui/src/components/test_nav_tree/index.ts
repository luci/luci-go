/* Copyright 2020 The LUCI Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { MobxLitElement } from '@adobe/lit-mobx';
import '@material/mwc-icon';
import { css, customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import { observable } from 'mobx';
import { TestNode } from '../../models/test_node';
import './test_nav_node';


/**
 * Display all test IDs in a folder-like tree structure.
 * Segments ending with /\W/ in test IDs are treated as "folders".
 * When a node has no sibling, it's collapsed into its parent.
 */
@customElement('tr-test-nav-tree')
export class TestNavTreeElement extends MobxLitElement {
  @observable.ref root?: TestNode;

  protected render() {
    return html`
      <div id="header" title=${this.root!.name}>${this.root!.name}</div>
      <div id="body">
        <div id="content">
          ${repeat(this.root!.children, (node) => node.name, (node) => html`
          <tr-test-nav-node
            .depth=${0}
            .node=${node}
          >
          </tr-test-nav-node>
          `)}
        </div>
      </div>
    `;
  }

  static styles = css`
    #header {
      background-color: #DDDDDD;
      padding: 2px 5px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }
    .expandable-header.selected {
      background-color: #66ccff;
    }

    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
      overflow-y: auto;
      height: 100%;
    }
    #content {
      grid-column: 1/3;
      grid-row: 1;
    }
  `;
}
