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
import { css, customElement, html } from 'lit-element';
import { classMap } from 'lit-html/directives/class-map';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction } from 'mobx';

import '../dot_spinner';
import './tvt_column_header';
import './test_variant_entry';
import { AppState, consumeAppState } from '../../context/app_state';
import { consumeConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { VARIANT_STATUS_CLASS_MAP } from '../../libs/constants';
import { consumer } from '../../libs/context';
import { reportErrorAsync } from '../../libs/error_handler';
import { createTVPropGetter, getPropKeyLabel, TestVariantStatus } from '../../services/resultdb';
import colorClasses from '../../styles/color_classes.css';
import commonStyle from '../../styles/common_style.css';
import { MiloBaseElement } from '../milo_base';
import { consumeTestVariantTableState, TestVariantTableState, VariantGroup } from './context';
import { TestVariantEntryElement } from './test_variant_entry';

/**
 * Displays test variants in a table.
 */
@customElement('milo-test-variants-table')
@consumer
export class TestVariantsTableElement extends MiloBaseElement {
  @observable.ref @consumeAppState() appState!: AppState;
  @observable.ref @consumeConfigsStore() configsStore!: UserConfigsStore;
  @observable.ref @consumeTestVariantTableState() tableState!: TestVariantTableState;

  @observable.ref hideTestName = false;
  @observable.ref showTimestamp = false;

  @computed private get columnGetters() {
    return this.tableState.columnKeys.map((col) => createTVPropGetter(col));
  }

  @computed private get columnWidths() {
    if (this.hideTestName && this.tableState.columnWidths.length > 0) {
      const ret = this.tableState.columnWidths.slice();
      ret.pop();
      return ret;
    }
    return this.tableState.columnWidths;
  }

  private getTvtColumns(columnWidths: readonly number[]) {
    return (
      '--tvt-columns: 24px ' +
      (this.showTimestamp ? '135px ' : '') +
      columnWidths.map((width) => width + 'px').join(' ') +
      ' 1fr'
    );
  }

  toggleAllVariants(expand: boolean) {
    this.shadowRoot!.querySelectorAll<TestVariantEntryElement>('milo-test-variant-entry').forEach(
      (e) => (e.expanded = expand)
    );
  }

  connectedCallback() {
    super.connectedCallback();

    // When a new test loader is received, load the first page.
    this.addDisposer(
      reaction(
        () => this.tableState.loadedFirstPage,
        () => {
          if (this.tableState.loadedFirstPage) {
            return;
          }
          reportErrorAsync(this, () => this.tableState.loadFirstPage())();
        },
        { fireImmediately: true }
      )
    );

    // Sync column width from the user config.
    this.addDisposer(
      reaction(
        () => this.configsStore.userConfigs.testResults.columnWidths,
        (columnWidths) => this.tableState.setColumnWidths(columnWidths),
        { fireImmediately: true }
      )
    );
  }

  private loadMore = reportErrorAsync(this, () => this.tableState.loadNextPage());

  private renderAllVariants() {
    return html`
      ${this.tableState.variantGroups.map((group) => this.renderVariantGroup(group))}
      <div id="variant-list-tail">
        ${this.tableState.testVariantCount === this.tableState.unfilteredTestVariantCount
          ? html`
              Showing ${this.tableState.testVariantCount} /
              ${this.tableState.unfilteredTestVariantCount}${this.tableState.loadedAllTestVariants ? '' : '+'} tests.
            `
          : html`
              Showing
              <i>${this.tableState.testVariantCount}</i>
              test${this.tableState.testVariantCount === 1 ? '' : 's'} that
              <i>match${this.tableState.testVariantCount === 1 ? 'es' : ''} the filter</i>, out of
              <i>${this.tableState.unfilteredTestVariantCount}${this.tableState.loadedAllTestVariants ? '' : '+'}</i>
              tests.
            `}
        <span
          class="active-text"
          style=${styleMap({
            display: !this.tableState.loadedAllTestVariants && this.tableState.readyToLoad ? '' : 'none',
          })}
          >${this.renderLoadMore()}</span
        >
      </div>
    `;
  }

  @observable private collapsedVariantGroups = new Set<string>();
  private renderVariantGroup(group: VariantGroup) {
    if (!this.tableState.enablesGrouping) {
      return repeat(
        group.variants,
        (v) => `${v.testId} ${v.variantHash}`,
        (v) => html`
          <milo-test-variant-entry
            .variant=${v}
            .columnGetters=${this.columnGetters}
            .expanded=${this.tableState.testVariantCount === 1}
            .hideTestName=${this.hideTestName}
            .showTimestamp=${this.showTimestamp}
            .historyUrl=${this.tableState.getHistoryUrl(v.testId)}
          ></milo-test-variant-entry>
        `
      );
    }

    const groupId = JSON.stringify(group.def);
    const expanded = !this.collapsedVariantGroups.has(groupId);
    return html`
      <div
        class=${classMap({
          expanded,
          empty: group.variants.length === 0,
          'group-header': true,
        })}
        @click=${() => {
          if (expanded) {
            this.collapsedVariantGroups.add(groupId);
          } else {
            this.collapsedVariantGroups.delete(groupId);
          }
        }}
      >
        <mwc-icon class="group-icon">${expanded ? 'expand_more' : 'chevron_right'}</mwc-icon>
        <div>
          <b>${group.variants.length} test variant${group.variants.length === 1 ? '' : 's'}:</b>
          ${group.def.map(
            ([k, v]) =>
              html`<span class="group-kv"
                ><span>${getPropKeyLabel(k)}=</span
                ><span class=${k === 'status' ? VARIANT_STATUS_CLASS_MAP[v as TestVariantStatus] : ''}>${v}</span></span
              >`
          )}
          ${group.note || ''}
        </div>
      </div>
      ${repeat(
        expanded ? group.variants : [],
        (v) => `${v.testId} ${v.variantHash}`,
        (v) => html`
          <milo-test-variant-entry
            .variant=${v}
            .columnGetters=${this.columnGetters}
            .expanded=${this.tableState.testVariantCount === 1}
            .hideTestName=${this.hideTestName}
            .showTimestamp=${this.showTimestamp}
            .historyUrl=${this.tableState.getHistoryUrl(v.testId)}
          ></milo-test-variant-entry>
        `
      )}
    `;
  }

  private renderLoadMore() {
    const state = this.tableState;
    return html`
      <span style=${styleMap({ display: state.isLoading ?? true ? 'none' : '' })} @click=${() => this.loadMore()}>
        [load more]
      </span>
      <span
        style=${styleMap({
          display: state.isLoading ?? true ? '' : 'none',
          cursor: 'initial',
        })}
      >
        loading <milo-dot-spinner></milo-dot-spinner>
      </span>
    `;
  }

  private tableHeaderEle?: HTMLElement;
  protected updated() {
    this.tableHeaderEle = this.shadowRoot!.getElementById('table-header')!;
  }

  protected render() {
    return html`
      <div style=${this.getTvtColumns(this.columnWidths)}>
        <div id="table-header">
          <div><!-- Expand toggle --></div>
          <milo-tvt-column-header
            .propKey=${'status'}
            .label=${/* invis char */ '\u2002' + 'S'}
            .canHide=${false}
          ></milo-tvt-column-header>
          ${this.showTimestamp
            ? html`
                <milo-tvt-column-header
                  .propKey=${'timestamp'}
                  .label=${'Date Time'}
                  .canHide=${false}
                ></milo-tvt-column-header>
              `
            : ''}
          ${this.tableState.columnKeys.map(
            (col, i) => html`<milo-tvt-column-header
              .colIndex=${
                // When hiding test name, don't make the last column resizable.
                this.tableState.columnKeys.length - 1 === i && this.hideTestName ? undefined : i
              }
              .resizeTo=${(newWidth: number, finalized: boolean) => {
                if (!finalized) {
                  const newColWidths = this.columnWidths.slice();
                  newColWidths[i] = newWidth;
                  // Update the style directly so lit-element doesn't need to
                  // re-render the component frequently.
                  // Live updating the width of the entire column can cause a bit
                  // of lag when there are many rows. Live updating just the
                  // column header is good enough.
                  this.tableHeaderEle?.style.setProperty('--tvt-columns', this.getTvtColumns(newColWidths));
                  return;
                }

                this.tableHeaderEle?.style.removeProperty('--tvt-columns');
                this.configsStore.userConfigs.testResults.columnWidths[col] = newWidth;
              }}
              .propKey=${col}
              .label=${getPropKeyLabel(col)}
            ></milo-tvt-column-header>`
          )}
          ${this.hideTestName
            ? ''
            : html`
                <milo-tvt-column-header .propKey=${'name'} .label=${'Name'} .canHide=${false} .canGroup=${false}>
                </milo-tvt-column-header>
              `}
        </div>
        <div id="test-variant-list" tabindex="0">${this.renderAllVariants()}</div>
      </div>
    `;
  }

  static styles = [
    commonStyle,
    colorClasses,
    css`
      :host {
        display: block;
        --tvt-top-offset: 0px;
      }

      #table-header {
        display: grid;
        grid-template-columns: 24px var(--tvt-columns);
        grid-gap: 5px;
        line-height: 24px;
        padding: 2px 2px 2px 10px;
        font-weight: bold;
        position: sticky;
        top: var(--tvt-top-offset);
        border-top: 1px solid var(--divider-color);
        border-bottom: 1px solid var(--divider-color);
        background-color: var(--block-background-color);
        z-index: 2;
      }

      #no-invocation {
        padding: 10px;
      }
      #test-variant-list > * {
        padding-left: 10px;
      }
      milo-test-variant-entry {
        margin: 2px 0px;
      }

      #integration-hint {
        border-bottom: 1px solid var(--divider-color);
        padding: 0 0 5px 15px;
      }

      .group-header {
        display: grid;
        grid-template-columns: auto auto 1fr;
        grid-gap: 5px;
        padding: 2px 2px 2px 10px;
        position: sticky;
        top: calc(var(--tvt-top-offset) + 29px);
        font-size: 14px;
        background-color: var(--block-background-color);
        border-top: 1px solid var(--divider-color);
        cursor: pointer;
        line-height: 24px;
        z-index: 1;
      }
      .group-header:first-child {
        top: calc(var(--tvt-top-offset) + 30px);
        border-top: none;
      }
      .group-header.expanded:not(.empty) {
        border-bottom: 1px solid var(--divider-color);
      }
      .group-kv:not(:last-child)::after {
        content: ', ';
      }
      .group-kv > span:first-child {
        color: var(--light-text-color);
      }
      .group-kv > span:nth-child(2) {
        font-weight: 500;
        font-style: italic;
      }

      .inline-icon {
        --mdc-icon-size: 1.2em;
        vertical-align: bottom;
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
