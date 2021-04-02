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

import { consumeInvocationState, InvocationState } from '../../context/invocation_state';

@customElement('milo-tvt-column-header')
@consumeInvocationState
export class TestVariantsTableColumnHeader extends MobxLitElement {
  @observable.ref invocationState!: InvocationState;

  @observable.ref propKey!: string;
  @observable.ref label!: string;
  @observable.ref canGroup = true;
  @observable.ref canHide = true;

  @observable.ref private menuIsOpen = false;

  private removeKey(oldKeys: string[]): string[] {
    const keys = oldKeys.slice();
    const i = keys.findIndex((k) => k === this.propKey || k === '-' + this.propKey);
    if (i > -1) {
      keys.splice(i, 1);
    }
    return keys;
  }

  sortColumn(ascending: boolean) {
    const newSortingKeys = this.removeKey(this.invocationState.sortingKeys);
    newSortingKeys.unshift((ascending ? '' : '-') + this.propKey);
    this.invocationState.sortingKeysParam = newSortingKeys;
  }

  groupRows() {
    this.hideColumn();
    const newGroupingKeys = this.removeKey(this.invocationState.groupingKeys);
    newGroupingKeys.unshift(this.propKey);
    this.invocationState.groupingKeysParam = newGroupingKeys;
  }

  hideColumn() {
    const newColumnKeys = this.removeKey(this.invocationState.displayedColumns);
    this.invocationState.columnsParam = newColumnKeys;
  }

  protected render() {
    return html`
      <div
        @click=${() => (this.menuIsOpen = !this.menuIsOpen)}
        @selected=${() => (this.menuIsOpen = false)}
        title=${this.propKey}
      >
        ${this.label}
      </div>
      <mwc-menu absolute x="0" y="10" ?open=${this.menuIsOpen} @closed=${() => (this.menuIsOpen = false)}>
        <mwc-list-item @click=${() => this.sortColumn(true)}>Sort in ascending order</mwc-list-item>
        <mwc-list-item @click=${() => this.sortColumn(false)}>Sort in descending order</mwc-list-item>
        <mwc-list-item style=${styleMap({ display: this.canGroup ? '' : 'none' })} @click=${() => this.groupRows()}>
          Group by ${this.label}
        </mwc-list-item>
        <mwc-list-item style=${styleMap({ display: this.canHide ? '' : 'none' })} @click=${() => this.hideColumn()}>
          Hide column
        </mwc-list-item>
      </mwc-menu>
    `;
  }

  static styles = css`
    :host {
      display: block;
      position: relative;
    }

    div {
      display: inline-block;
      cursor: pointer;
      color: var(--active-text-color);
    }
  `;
}
