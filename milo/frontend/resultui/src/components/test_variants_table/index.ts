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
import { deepEqual } from 'fast-equals';
import { css, customElement, html } from 'lit-element';
import { classMap } from 'lit-html/directives/class-map';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction } from 'mobx';

import '../dot_spinner';
import './tvt_column_header';
import './test_variant_entry';
import { AppState, consumeAppState } from '../../context/app_state';
import { consumeInvocationState, InvocationState } from '../../context/invocation_state';
import { consumeConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { VARIANT_STATUS_CLASS_MAP } from '../../libs/constants';
import { consumer } from '../../libs/context';
import { reportErrorAsync } from '../../libs/error_handler';
import { getPropKeyLabel, TestVariant, TestVariantStatus } from '../../services/resultdb';
import colorClasses from '../../styles/color_classes.css';
import commonStyle from '../../styles/common_style.css';
import { MiloBaseElement } from '../milo_base';
import { TestVariantEntryElement } from './test_variant_entry';

/**
 * Displays test variants in a table.
 */
@customElement('milo-test-variants-table')
@consumer
export class TestVariantsTableElement extends MiloBaseElement {
  @observable.ref @consumeAppState() appState!: AppState;
  @observable.ref @consumeConfigsStore() configsStore!: UserConfigsStore;
  @observable.ref @consumeInvocationState() invocationState!: InvocationState;

  @computed private get hasCustomGroupingKey() {
    return !deepEqual(this.invocationState.groupingKeys, ['status']);
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
        () => this.invocationState.testLoader,
        (testLoader) => reportErrorAsync(this, async () => testLoader?.loadFirstPageOfTestVariants())(),
        { fireImmediately: true }
      )
    );
  }

  private loadMore = reportErrorAsync(this, async () => this.invocationState.testLoader?.loadNextTestVariants());

  private renderAllVariants() {
    const testLoader = this.invocationState.testLoader;
    const groupers = this.invocationState.groupers;
    return html`
      ${
        // Indicates that there are no unexpected test variants.
        testLoader?.loadedAllUnexpectedVariants && testLoader.unexpectedTestVariants.length === 0
          ? this.renderVariantGroup([['status', TestVariantStatus.UNEXPECTED]], [])
          : ''
      }
      ${(testLoader?.groupedNonExpectedVariants || []).map((group) =>
        this.renderVariantGroup(
          groupers.map(([key, getter]) => [key, getter(group[0])]),
          group
        )
      )}
      ${this.renderVariantGroup(
        [['status', TestVariantStatus.EXPECTED]],
        testLoader?.expectedTestVariants || [],
        this.hasCustomGroupingKey ? html`<b>note: custom grouping doesn't apply to expected tests</b>` : ''
      )}
      <div id="variant-list-tail">
        ${testLoader?.testVariantCount === testLoader?.unfilteredTestVariantCount
          ? html`
              Showing ${testLoader?.testVariantCount || 0} /
              ${testLoader?.unfilteredTestVariantCount || 0}${testLoader?.loadedAllVariants ? '' : '+'} tests.
            `
          : html`
              Showing
              <i>${testLoader?.testVariantCount || 0}</i>
              test${testLoader?.testVariantCount === 1 ? '' : 's'} that
              <i>match${testLoader?.testVariantCount === 1 ? 'es' : ''} the filter</i>, out of
              <i>${testLoader?.unfilteredTestVariantCount || 0}${testLoader?.loadedAllVariants ? '' : '+'}</i> tests.
            `}
        <span
          class="active-text"
          style=${styleMap({ display: this.invocationState.testLoader?.loadedAllVariants ?? true ? 'none' : '' })}
          >${this.renderLoadMore()}</span
        >
      </div>
    `;
  }

  @observable private collapsedVariantGroups = new Set<string>();
  private renderVariantGroup(groupDef: [string, unknown][], variants: TestVariant[], note: unknown = null) {
    const groupId = JSON.stringify(groupDef);
    const expanded = !this.collapsedVariantGroups.has(groupId);
    return html`
      <div
        class=${classMap({
          expanded,
          empty: variants.length === 0,
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
          <b>${variants.length} test variant${variants.length === 1 ? '' : 's'}:</b>
          ${groupDef.map(
            ([k, v]) =>
              html`<span class="group-kv"
                ><span>${getPropKeyLabel(k)}=</span
                ><span class=${k === 'status' ? VARIANT_STATUS_CLASS_MAP[v as TestVariantStatus] : ''}>${v}</span></span
              >`
          )}
          ${note}
        </div>
      </div>
      ${repeat(
        expanded ? variants : [],
        (v) => `${v.testId} ${v.variantHash}`,
        (v) => html`
          <milo-test-variant-entry
            .variant=${v}
            .columnGetters=${this.invocationState.displayedColumnGetters}
            .expanded=${this.invocationState.testLoader?.testVariantCount === 1}
          ></milo-test-variant-entry>
        `
      )}
    `;
  }

  private renderLoadMore() {
    const state = this.invocationState;
    return html`
      <span
        style=${styleMap({ display: state.testLoader?.isLoading ?? true ? 'none' : '' })}
        @click=${() => this.loadMore()}
      >
        [load more]
      </span>
      <span
        style=${styleMap({
          display: state.testLoader?.isLoading ?? true ? '' : 'none',
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
      <div id="table-header">
        <div><!-- Expand toggle --></div>
        <milo-tvt-column-header
          .propKey=${'status'}
          .label=${/* invis char */ '\u2002' + 'S'}
          .canHide=${false}
        ></milo-tvt-column-header>
        ${this.invocationState.displayedColumns.map(
          (col, i) => html`<milo-tvt-column-header
            .colIndex=${i}
            .resizeTo=${(newWidth: number, finalized: boolean) => {
              if (!finalized) {
                const newColWidths = this.invocationState.columnWidths.slice();
                newColWidths[i] = newWidth;
                // Update the style directly so lit-element doesn't need to
                // re-render the component frequently.
                // Live updating the width of the entire column can cause a bit
                // of lag when there are many rows. Live updating just the
                // column header is good enough.
                this.tableHeaderEle?.style.setProperty('--columns', newColWidths.map((w) => w + 'px').join(' '));
                return;
              }

              this.tableHeaderEle?.style.removeProperty('--columns');
              this.configsStore.userConfigs.testResults.columnWidths[col] = newWidth;
            }}
            .propKey=${col}
            .label=${getPropKeyLabel(col)}
          ></milo-tvt-column-header>`
        )}
        <milo-tvt-column-header .propKey=${'name'} .label=${'Name'} .canHide=${false} .canGroup=${false}>
        </milo-tvt-column-header>
      </div>
      <div id="test-variant-list" tabindex="0">${this.renderAllVariants()}</div>
    `;
  }

  static styles = [
    commonStyle,
    colorClasses,
    css`
      #table-header {
        display: grid;
        grid-template-columns: 24px 24px var(--columns) 1fr;
        grid-gap: 5px;
        line-height: 24px;
        padding: 2px 2px 2px 10px;
        font-weight: bold;
        position: sticky;
        top: 39px;
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
        top: 67px;
        font-size: 14px;
        background-color: var(--block-background-color);
        border-top: 1px solid var(--divider-color);
        cursor: pointer;
        line-height: 24px;
        z-index: 1;
      }
      .group-header:first-child {
        top: 68px;
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
