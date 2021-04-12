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
import { consumeInvocationState, InvocationState } from '../../context/invocation_state';
import { consumeConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { GA_ACTIONS, GA_CATEGORIES, generateRandomLabel, trackEvent } from '../../libs/analytics_utils';
import { VARIANT_STATUS_CLASS_MAP } from '../../libs/constants';
import { TestVariant, TestVariantStatus } from '../../services/resultdb';
import colorClasses from '../../styles/color_classes.css';
import { MiloBaseElement } from '../milo_base';
import { TestVariantEntryElement } from './test_variant_entry';

function getPropKeyLabel(key: string) {
  // If the key has the format of '{type}.{value}', hide the '{type}.' prefix.
  return key.split('.', 2)[1] ?? key;
}

/**
 * Displays test variants in a table.
 */
@customElement('milo-test-variants-table')
@consumeInvocationState
@consumeConfigsStore
@consumeAppState
export class TestVariantsTableElement extends MiloBaseElement {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref invocationState!: InvocationState;

  private sentLoadingTimeToGA = false;

  toggleAllVariants(expand: boolean) {
    this.shadowRoot!.querySelectorAll<TestVariantEntryElement>('milo-test-variant-entry').forEach(
      (e) => (e.expanded = expand)
    );
  }

  connectedCallback() {
    super.connectedCallback();

    // If first page of test results has already been loaded when connected
    // (happens when users switch tabs), we don't want to track the loading
    // time (only a few ms in this case)
    if (this.invocationState.testLoader?.firstPageLoaded) {
      this.sentLoadingTimeToGA = true;
    }

    // When a new test loader is received, load the first page and reset the
    // selected node.
    this.addDisposer(
      reaction(
        () => this.invocationState.testLoader,
        (testLoader) => {
          if (!testLoader) {
            return;
          }
          // The previous instance of the test results tab could've triggered
          // the loading operation already. In that case we don't want to load
          // more test results.
          if (!testLoader.firstRequestSent) {
            this.loadMore();
          }
        },
        { fireImmediately: true }
      )
    );
  }

  private async loadMore(untilStatus?: TestVariantStatus) {
    try {
      await this.invocationState.testLoader?.loadNextTestVariants(untilStatus);
    } catch (e) {
      this.dispatchEvent(
        new ErrorEvent('error', {
          error: e,
          message: e.toString(),
          composed: true,
          bubbles: true,
        })
      );
    }
  }

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
      ${this.renderVariantGroup([['status', TestVariantStatus.EXPECTED]], testLoader?.expectedTestVariants || [])}
      <div id="variant-list-tail">
        Showing ${testLoader?.testVariantCount || 0} /
        ${testLoader?.unfilteredTestVariantCount || 0}${testLoader?.loadedAllVariants ? '' : '+'} tests.
        <span
          class="active-text"
          style=${styleMap({ display: this.invocationState.testLoader?.loadedAllVariants ?? true ? 'none' : '' })}
          >${this.renderLoadMore()}</span
        >
      </div>
    `;
  }

  private sendLoadingTimeToGA = () => {
    if (this.sentLoadingTimeToGA) {
      return;
    }
    this.sentLoadingTimeToGA = true;
    trackEvent(
      GA_CATEGORIES.TEST_RESULTS_TAB,
      GA_ACTIONS.LOADING_TIME,
      generateRandomLabel(this.gaLabelPrefix),
      Date.now() - this.appState.tabSelectionTime
    );
  };

  @computed private get gaLabelPrefix() {
    return 'testresults_' + this.invocationState.invocationId;
  }

  private variantExpandedCallback = () => {
    trackEvent(GA_CATEGORIES.TEST_RESULTS_TAB, GA_ACTIONS.EXPAND_ENTRY, `${this.gaLabelPrefix}_${VISIT_ID}`, 1);
  };

  @observable private collapsedVariantGroups = new Set<string>();
  private renderVariantGroup(groupDef: [string, unknown][], variants: TestVariant[]) {
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
            .prerender=${true}
            .expandedCallback=${this.variantExpandedCallback}
            .renderedCallback=${this.sendLoadingTimeToGA}
          ></milo-test-variant-entry>
        `
      )}
    `;
  }

  private renderLoadMore(forStatus?: TestVariantStatus) {
    const state = this.invocationState;
    return html`
      <span
        style=${styleMap({ display: state.testLoader?.isLoading ?? true ? 'none' : '' })}
        @click=${(e: Event) => {
          this.loadMore(forStatus);
          e.stopPropagation();
        }}
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
      <milo-hotkey
        key="space,shift+space,up,down,pageup,pagedown"
        style="display: none;"
        .handler=${() => this.shadowRoot!.getElementById('test-variant-list')!.focus()}
      ></milo-hotkey>
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
              this.configsStore.save();
            }}
            .propKey=${col}
            .label=${getPropKeyLabel(col)}
          ></milo-tvt-column-header>`
        )}
        <milo-tvt-column-header .propKey=${'name'} .label=${'Name'} .canHide=${false} .canGroup=${false}>
        </milo-tvt-column-header>
      </div>
      <milo-lazy-list id="test-variant-list" .growth=${300} tabindex="-1">${this.renderAllVariants()}</milo-lazy-list>
    `;
  }

  static styles = [
    colorClasses,
    css`
      :host {
        overflow: hidden;
        display: grid;
        grid-template-rows: auto 1fr;
      }

      #table-header {
        display: grid;
        grid-template-columns: 24px 24px var(--columns) 1fr;
        grid-gap: 5px;
        line-height: 24px;
        padding: 2px 2px 2px 10px;
        font-weight: bold;
        border-bottom: 1px solid var(--divider-color);
        background-color: var(--block-background-color);
      }

      #table-body {
        overflow-y: hidden;
      }
      milo-lazy-list > * {
        padding-left: 10px;
      }
      #no-invocation {
        padding: 10px;
      }
      #test-variant-list {
        overflow-y: auto;
        outline: none;
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
        font-size: 14px;
        background-color: var(--block-background-color);
        border-top: 1px solid var(--divider-color);
        top: -1px;
        cursor: pointer;
        user-select: none;
        line-height: 24px;
      }
      .group-header:first-child {
        top: 0px;
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

      .active-text {
        color: var(--active-text-color);
        cursor: pointer;
        font-size: 14px;
        font-weight: normal;
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
