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

import '@material/mwc-button';
import '@material/mwc-icon';
import '@/generic_libs/components/dot_spinner';
import '@/common/components/column_header';
import './test_history_details_entry';

import { Interpolation, Theme } from '@emotion/react';
import { css, html } from 'lit';
import { customElement } from 'lit/decorators.js';
import { repeat } from 'lit/directives/repeat.js';
import { styleMap } from 'lit/directives/style-map.js';
import { computed, makeObservable, observable, reaction } from 'mobx';
import { ForwardedRef, forwardRef } from 'react';

import {
  createTVPropGetter,
  getPropKeyLabel,
} from '@/common/services/resultdb';
import { consumeStore, StoreInstance } from '@/common/store';
import { colorClasses, commonStyles } from '@/common/styles/stylesheets';
import { MobxExtLitElement } from '@/generic_libs/components/lit_mobx_ext';
import { ReactLitBridge } from '@/generic_libs/components/react_lit_element';
import { reportErrorAsync } from '@/generic_libs/tools/error_handler';
import { consumer } from '@/generic_libs/tools/lit_context';
import { assertNonNullable } from '@/generic_libs/tools/utils';

import { TestHistoryDetailsEntryElement } from './test_history_details_entry';

/**
 * Displays test variants in a table.
 */
@customElement('milo-th-details-table')
@consumer
export class TestHistoryDetailsTableElement extends MobxExtLitElement {
  @observable.ref @consumeStore() store!: StoreInstance;
  @computed get pageState() {
    return this.store.testHistoryPage;
  }

  @computed private get columnGetters() {
    return this.pageState.columnKeys.map((col) => createTVPropGetter(col));
  }

  @computed private get columnWidths() {
    if (this.pageState.columnWidths.length > 0) {
      const ret = this.pageState.columnWidths.slice();
      ret.pop();
      return ret;
    }
    return this.pageState.columnWidths;
  }

  constructor() {
    super();
    makeObservable(this);
  }

  private getThdtColumns(columnWidths: readonly number[]) {
    return (
      '24px 135px 250px 400px' +
      columnWidths.map((width) => width + 'px').join(' ') +
      ' 1fr'
    );
  }

  private allVariantsWereExpanded = false;
  toggleAllVariants(expand?: boolean) {
    this.allVariantsWereExpanded = expand ?? !this.allVariantsWereExpanded;
    this.shadowRoot!.querySelectorAll<TestHistoryDetailsEntryElement>(
      'milo-test-history-details-entry',
    ).forEach((e) => (e.expanded = this.allVariantsWereExpanded));
  }

  connectedCallback() {
    super.connectedCallback();

    // When a new test loader is received, load the first page.
    this.addDisposer(
      reaction(
        () => this.pageState.entriesLoader,
        (entriesLoader) =>
          reportErrorAsync(
            this,
            async () => await entriesLoader?.loadFirstPage(),
          )(),
        { fireImmediately: true },
      ),
    );

    // Sync column width from the user config.
    this.addDisposer(
      reaction(
        () => this.store.userConfig.tests.columnWidths,
        (columnWidths) => this.pageState.setColumnWidths(columnWidths),
        { fireImmediately: true },
      ),
    );
  }

  private loadMore = reportErrorAsync(
    this,
    async () => await this.pageState.entriesLoader?.loadNextPage(),
  );

  private renderAllVariants() {
    const entryLoaders = this.pageState.entriesLoader;
    return html`
      ${repeat(
        this.pageState.verdictBundles,
        ({ verdict }) =>
          `${verdict.testId} ${verdict.variantHash} ${verdict.invocationId}`,
        (v) => html`
          <milo-test-history-details-entry
            .verdictBundle=${v}
            .columnGetters=${this.columnGetters}
            .expanded=${entryLoaders?.items.length === 1}
          ></milo-test-history-details-entry>
        `,
      )}
      <div id="variant-list-tail">
        Showing ${entryLoaders?.items.length || 0} /
        ${this.pageState.selectedTestVerdictCount}${entryLoaders?.loadedAll
          ? ''
          : '+'}
        tests.
        <span
          class="active-text"
          style=${styleMap({
            display: !(entryLoaders?.loadedAll ?? true) ? '' : 'none',
          })}
          >${this.renderLoadMore()}</span
        >
      </div>
    `;
  }
  private renderLoadMore() {
    const entriesLoader = this.pageState.entriesLoader;
    return html`
      <span
        style=${styleMap({
          display: (entriesLoader?.isLoading ?? true) ? 'none' : '',
        })}
        @click=${() => this.loadMore()}
      >
        [load more]
      </span>
      <span
        style=${styleMap({
          display: (entriesLoader?.isLoading ?? true) ? '' : 'none',
          cursor: 'initial',
        })}
      >
        loading <milo-dot-spinner></milo-dot-spinner>
      </span>
    `;
  }

  private tableHeaderEle?: HTMLElement;
  protected updated() {
    this.tableHeaderEle = assertNonNullable(
      this.shadowRoot?.getElementById('table-header'),
    );
  }

  /**
   * Generate a sortByColumn callback for the given column.
   */
  private sortByColumnFn(col: string) {
    return (ascending: boolean) => {
      const matchingKeys = [col, `-${col}`];
      const newKeys = this.pageState.sortingKeys.filter(
        (key) => !matchingKeys.includes(key),
      );
      newKeys.unshift((ascending ? '' : '-') + col);
      this.pageState.setSortingKeys(newKeys);
    };
  }

  protected render() {
    return html`
      <div style="--thdt-columns: ${this.getThdtColumns(this.columnWidths)}">
        <div id="table-header">
          <div><!-- Expand toggle --></div>
          <milo-column-header
            .label=${/* invis char */ '\u2002' + 'S'}
            .tooltip=${'status'}
            .sortByColumn=${this.sortByColumnFn('status')}
          ></milo-column-header>
          <milo-column-header
            .label=${'Timestamp'}
            .tooltip=${'partitionTime'}
          ></milo-column-header>
          <milo-column-header
            .label=${'Invocation'}
            .tooltip=${'invocationId'}
          ></milo-column-header>
          <milo-column-header
            .label=${'Changelists'}
            .tooltip=${'changelists'}
          ></milo-column-header>
          ${this.pageState.columnKeys.map(
            (col, i) =>
              html`<milo-column-header
                .label=${getPropKeyLabel(col)}
                .tooltip=${col}
                .resizeColumn=${
                  // Don't make the last column resizable.
                  this.pageState.columnKeys.length - 1 === i
                    ? undefined
                    : (delta: number, finalized: boolean) => {
                        if (!finalized) {
                          const newColWidths = this.columnWidths.slice();
                          newColWidths[i] += delta;
                          // Update the style directly so lit-element doesn't need to
                          // re-render the component frequently.
                          // Live updating the width of the entire column can cause a bit
                          // of lag when there are many rows. Live updating just the
                          // column header is good enough.
                          this.tableHeaderEle?.style.setProperty(
                            '--thdt-columns',
                            this.getThdtColumns(newColWidths),
                          );
                          return;
                        }

                        this.tableHeaderEle?.style.removeProperty(
                          '--thdt-columns',
                        );
                        this.store.userConfig.tests.setColumWidth(
                          col,
                          this.columnWidths[i] + delta,
                        );
                      }
                }
                .sortByColumn=${this.sortByColumnFn(col)}
              ></milo-column-header>`,
          )}
        </div>
        <div id="test-variant-list" tabindex="0">
          ${this.renderAllVariants()}
        </div>
      </div>
    `;
  }

  static styles = [
    commonStyles,
    colorClasses,
    css`
      :host {
        display: block;
        --thdt-top-offset: 0px;
      }

      #table-header {
        display: grid;
        grid-template-columns: 24px var(--thdt-columns);
        grid-gap: 5px;
        line-height: 24px;
        padding: 2px 2px 2px 10px;
        font-weight: bold;
        position: sticky;
        top: var(--thdt-top-offset);
        border-top: 1px solid var(--divider-color);
        border-bottom: 1px solid var(--divider-color);
        background-color: var(--block-background-color);
        z-index: 2;
      }

      #test-variant-list > * {
        padding-left: 10px;
      }
      milo-test-history-details-entry {
        margin: 2px 0px;
      }

      #variant-list-tail {
        padding: 5px 0 5px 15px;
      }
      #variant-list-tail:not(:first-child) {
        border-top: 1px solid var(--divider-color);
      }
      #load {
        color: var(--active-text-color);
      }
    `,
  ];
}

declare global {
  // eslint-disable-next-line @typescript-eslint/no-namespace
  namespace JSX {
    interface IntrinsicElements {
      'milo-th-details-table': {
        css?: Interpolation<Theme>;
        ref?: ForwardedRef<TestHistoryDetailsTableElement>;
        class?: string;
      };
    }
  }
}

export interface DetailsTableProps {
  readonly css?: Interpolation<Theme>;
  readonly className?: string;
}

export const DetailsTable = forwardRef(
  (
    props: DetailsTableProps,
    ref: ForwardedRef<TestHistoryDetailsTableElement>,
  ) => {
    return (
      <ReactLitBridge>
        <milo-th-details-table {...props} ref={ref} class={props.className} />
      </ReactLitBridge>
    );
  },
);
DetailsTable.displayName = 'DetailsTable';
