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

import '@material/mwc-menu';
import { MobxLitElement } from '@adobe/lit-mobx';
import { css, customElement, html } from 'lit-element';
import { styleMap } from 'lit-html/directives/style-map';
import { observable } from 'mobx';

import './drag_tracker';
import { consumer } from '../libs/context';
import {
  consumeTestVariantTableState,
  TestVariantTableState,
} from '../pages/test_results_tab/test_variants_table/context';
import commonStyle from '../styles/common_style.css';
import { DragEvent } from './drag_tracker';

@customElement('milo-tvt-column-header')
@consumer
export class TestVariantsTableColumnHeader extends MobxLitElement {
  @observable.ref
  @consumeTestVariantTableState()
  tableState!: TestVariantTableState;

  // Setting the colIndex also makes the column resizable.
  @observable.ref colIndex?: number;
  // finalized is set to true when the user stopped dragging.
  @observable.ref resizeTo = (_newWidth: number, _finalized: boolean) => {};

  @observable.ref propKey!: string;
  @observable.ref label!: string;
  @observable.ref canGroup = true;
  @observable.ref canHide = true;

  @observable.ref private menuIsOpen = false;

  private removeKey(oldKeys: readonly string[]): string[] {
    const keys = oldKeys.slice();
    const i = keys.findIndex((k) => k === this.propKey || k === '-' + this.propKey);
    if (i > -1) {
      keys.splice(i, 1);
    }
    return keys;
  }

  sortColumn(ascending: boolean) {
    const newSortingKeys = this.removeKey(this.tableState.sortingKeys);
    newSortingKeys.unshift((ascending ? '' : '-') + this.propKey);
    this.tableState.setSortingKeys(newSortingKeys);
  }

  groupRows() {
    this.hideColumn();
    const newGroupingKeys = this.removeKey(this.tableState.groupingKeys);
    newGroupingKeys.push(this.propKey);
    this.tableState.setGroupingKeys(newGroupingKeys);
  }

  hideColumn() {
    const newColumnKeys = this.removeKey(this.tableState.columnKeys);
    this.tableState.setColumnKeys(newColumnKeys);
  }

  private renderResizer() {
    if (this.colIndex === undefined) {
      return html``;
    }
    const startWidth = this.tableState.columnWidths[this.colIndex];
    let currentWidth = startWidth;
    return html`
      <milo-drag-tracker
        id="resizer"
        @drag=${(e: DragEvent) => {
          currentWidth = Math.max(startWidth + e.detail.dx, 24);
          this.resizeTo(currentWidth, false);
        }}
        @dragend=${() => this.resizeTo(currentWidth, true)}
      ></milo-drag-tracker>
    `;
  }

  protected render() {
    return html`
      <div id="prop-label" title=${this.propKey} @click=${() => (this.menuIsOpen = !this.menuIsOpen)}>
        ${this.label}
      </div>
      <div id="padding"></div>
      ${this.renderResizer()}
      <mwc-menu x="0" y="20" ?open=${this.menuIsOpen} @closed=${() => (this.menuIsOpen = false)}>
        <mwc-list-item @click=${() => this.sortColumn(true)}>Sort in ascending order</mwc-list-item>
        <mwc-list-item @click=${() => this.sortColumn(false)}>Sort in descending order</mwc-list-item>
        <mwc-list-item
          style=${styleMap({ display: this.tableState.enablesGrouping && this.canGroup ? '' : 'none' })}
          @click=${() => this.groupRows()}
        >
          Group by ${this.label}
        </mwc-list-item>
        <mwc-list-item style=${styleMap({ display: this.canHide ? '' : 'none' })} @click=${() => this.hideColumn()}>
          Hide column
        </mwc-list-item>
      </mwc-menu>
    `;
  }

  static styles = [
    commonStyle,
    css`
      :host {
        display: flex;
        position: relative;
      }

      #prop-label {
        flex: 0 1 auto;
        cursor: pointer;
        color: var(--active-text-color);
        overflow: hidden;
        text-overflow: ellipsis;
      }
      #padding {
        flex: 1 1 auto;
      }
      #resizer {
        flex: 0 0 auto;
        background: linear-gradient(var(--divider-color), var(--divider-color)) 2px 0/1px 100% no-repeat;
        width: 5px;
        cursor: col-resize;
      }
    `,
  ];
}
