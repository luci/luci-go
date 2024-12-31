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

import { Interpolation, Theme } from '@emotion/react';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';

import { consumeStore, StoreInstance } from '@/common/store';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { consumer } from '@/generic_libs/tools/lit_context';

import { CELL_SIZE, X_AXIS_HEIGHT } from './constants';

@customElement('milo-th-variant-def-table')
@consumer
export class TestHistoryVariantDefTableElement extends MobxExtLitElement {
  @observable.ref @consumeStore() store!: StoreInstance;
  @computed get pageState() {
    return this.store.testHistoryPage;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    return html`
      <table>
        <thead>
          ${this.pageState.criticalVariantKeys.map((k) => html`<th>${k}</th>`)}
        </thead>
        <tbody>
          ${this.pageState.filteredVariants.map(
            ([_, v]) => html`
              <tr>
                ${this.pageState.criticalVariantKeys.map(
                  (k) => html`<td>${v.def[k] || ''}</td>`,
                )}
              </tr>
            `,
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
      white-space: nowrap;
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

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-th-variant-def-table': {
        css?: Interpolation<Theme>;
        class?: string;
      };
    }
  }
}

export interface VariantDefTableProps {
  readonly css?: Interpolation<Theme>;
  readonly className?: string;
}

export function VariantDefTable(props: VariantDefTableProps) {
  return <milo-th-variant-def-table {...props} class={props.className} />;
}
