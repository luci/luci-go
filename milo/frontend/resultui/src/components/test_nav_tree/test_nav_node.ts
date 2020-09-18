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
import { classMap } from 'lit-html/directives/class-map';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable } from 'mobx';

import { TestNode } from '../../models/test_node';
import { consumeTreeState, TestNavTreeState } from './context';


/**
 * A node in milo-test-nav-tree. Collapsed by default.
 */
export class TestNavNodeElement extends MobxLitElement {
  @observable.ref depth = 0;
  @observable.ref node!: TestNode;
  @observable.ref treeState!: TestNavTreeState;

  @observable.ref private _expanded = false;
  @computed get expanded() {
    return this._expanded;
  }
  set expanded(newVal: boolean) {
    this._expanded = newVal;
    // Always render the content once it was expanded so the descendants' states
    // don't get reset after the node is collapsed.
    this.shouldRenderContent = this.shouldRenderContent || newVal;
  }

  @observable.ref private shouldRenderContent = false;

  @computed private get isSelected() { return this.treeState.selectedChildNode === this.node; }

  private renderBody() {
    if (!this.shouldRenderContent) {
      return html``;
    }
    return html`
      <div id="content-ruler" style=${styleMap({left: `${this.depth * 10}px`})}></div>
      <div
        id="content"
        style=${styleMap({ display: this.expanded ? '' : 'none' })}
      >
        ${repeat(this.node.children, (node) => node.name, (node) => html`
        <milo-test-nav-node
          .depth=${this.depth + 1}
          .node=${node}
        >
        </milo-test-nav-node>
        `)}
      </div>
    `;
  }

  protected render() {
    return html`
      <div
        class=${classMap({
          'selected': this.isSelected,
          'expandable-header': true,
        })}
        style=${styleMap({
          'padding-left': `${this.depth * 10}px`,
        })}
      >
        <mwc-icon
          class="expand-toggle"
          style=${styleMap({ display: this.node.children.length === 0 ? 'none' : '' })}
          @click=${() => this.expanded = !this.expanded}
        >
          ${this.expanded ? 'expand_more' : 'chevron_right'}
        </mwc-icon>
        <span
          class="one-line-content name-label"
          style=${styleMap({
            'grid-column': this.node.children.length === 0 ? '1/3' : '',
            'padding-left': this.node.children.length === 0 ? '8px' : '',
          })}
          title=${this.node.name}
          @click=${() => this.treeState.selectedChildNode = this.isSelected ? null : this.node}}
        >${this.node!.name}</span>
      </div>
      <div id="body">${this.renderBody()}</div>
    `;
  }

  static styles = css`
    mwc-icon {
      --mwc-icon-size: 1em;
    }

    .expandable-header {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-template-rows: 24px;
      user-select: none;
    }
    .expandable-header .expand-toggle {
      cursor: pointer;
      grid-row: 1;
      grid-column: 1;
    }
    .expandable-header .one-line-content {
      grid-row: 1;
      grid-column: 2;
      font-size: 16px;
      line-height: 24px;
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }
    .expandable-header.selected {
      background-color: var(--light-active-color);
    }

    .name-label {
      overflow: hidden;
      white-space: nowrap;
      text-overflow: ellipsis;
    }

    #body {
      display: grid;
      grid-template-columns: 24px 1fr;
      grid-gap: 5px;
    }
    #content-ruler {
      position: relative;
      border-left: 1px solid;
      border-color: var(--divider-color);
      width: 1px;
      height: 100%;
      margin-left: 11.5px;
      grid-column: 1;
      grid-row: 1;
    }
    #content {
      grid-column: 1/3;
      grid-row: 1;
    }
  `;
}

customElement('milo-test-nav-node')(
  consumeTreeState(
    TestNavNodeElement,
  ),
);
