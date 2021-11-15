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

import { css, customElement, html } from 'lit-element';
import { comparer, computed, observable } from 'mobx';

import { MiloBaseElement } from '../../components/milo_base';
import { consumeTestHistoryPageState, TestHistoryPageState } from '../../context/test_history_page_state';
import { consumer } from '../../libs/context';
import { getCriticalVariantKeys } from '../../services/resultdb';
import { CELL_SIZE, X_AXIS_HEIGHT } from './constants';

@customElement('milo-th-variant-def-table')
@consumer
export class TestHistoryVariantDefTableElement extends MiloBaseElement {
  @observable @consumeTestHistoryPageState() pageState!: TestHistoryPageState;

  @computed private get variants() {
    return this.pageState.testHistoryLoader?.variants || [];
  }

  @computed({ equals: comparer.shallow }) private get criticalVariantKeys() {
    return getCriticalVariantKeys(this.variants.map(([_, v]) => v));
  }

  protected render() {
    return html`
      <table>
        <thead>
          ${this.criticalVariantKeys.map((k) => html`<th>${k}</th>`)}
        </thead>
        <tbody>
          ${this.variants.map(
            ([_, v]) => html`
              <tr>
                ${this.criticalVariantKeys.map((k) => html`<td>${v.def[k] || ''}</td>`)}
              </tr>
            `
          )}
        </tbody>
      </table>
    `;
  }

  static styles = css`
    :host {
      display: block;
    }

    table {
      border-spacing: 0;
    }
    table * {
      padding: 0;
    }
    thead {
      height: ${X_AXIS_HEIGHT}px;
    }
    tr,
    td {
      line-height: ${CELL_SIZE}px;
      height: ${CELL_SIZE}px;
    }
    tr:nth-child(odd) {
      background-color: var(--block-background-color);
    }
    td {
      text-align: center;
      padding: 0 2px;
    }
  `;
}
