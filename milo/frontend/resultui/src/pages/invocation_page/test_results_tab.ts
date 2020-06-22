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
import '@material/mwc-button';
import '@material/mwc-icon';
import { css, customElement, html } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction } from 'mobx';

import '../../components/left_panel';
import '../../components/test_filter';
import { TestFilter } from '../../components/test_filter';
import '../../components/test_nav_tree';
import '../../components/variant_entry';
import { VariantEntryElement } from '../../components/variant_entry';
import { consumeContext } from '../../libs/context';
import { ReadonlyVariant, TestNode, VariantStatus } from '../../models/test_node';
import { InvocationPageState } from './context';

/**
 * Display a list of test results.
 */
export class TestResultsTabElement extends MobxLitElement {
  @observable.ref pageState!: InvocationPageState;

  private disposers: Array<() => void> = [];
  private async loadNextPage() {
    try {
      await this.pageState.testLoader.loadNextPage();
    } catch (e) {
      this.dispatchEvent(new ErrorEvent('error', {
        error: e,
        message: e.toString(),
        composed: true,
        bubbles: true,
      }));
    }
  }

  @computed
  private get hasSingleVariant() {
    // this operation should be fast since the iterator is executed only when
    // there's only one test.
    return this.pageState.selectedNode.testCount === 1 && [...this.pageState.selectedNode.tests()].length === 1;
  }

  private toggleAllVariants(expand: boolean) {
    this.shadowRoot!.querySelectorAll<VariantEntryElement>('tr-variant-entry')
      .forEach((e) => e.expanded = expand);
  }

  connectedCallback() {
    super.connectedCallback();
    this.pageState.selectedTabId = 'test-results';

    // When a new test loader is received, load the first page and reset the
    // selected node.
    this.disposers.push(reaction(
      () => this.pageState.testLoader,
      (testLoader) => {
        this.loadNextPage();
        this.pageState.selectedNode = testLoader.node;
      },
      {fireImmediately: true},
    ));
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    for (const disposer of this.disposers) {
      disposer();
    }
  }

  private renderAllVariants() {
    const exoneratedVariants: ReadonlyVariant[] = [];
    const expectedVariants: ReadonlyVariant[] = [];
    const unexpectedVariants: ReadonlyVariant[] = [];
    const flakyVariants: ReadonlyVariant[] = [];
    for (const test of this.pageState.selectedNode.tests()) {
      for (const variant of test.variants) {
        switch (variant.status) {
          case VariantStatus.Exonerated:
            exoneratedVariants.push(variant);
            break;
          case VariantStatus.Expected:
            expectedVariants.push(variant);
            break;
          case VariantStatus.Unexpected:
            unexpectedVariants.push(variant);
            break;
          case VariantStatus.Flaky:
            flakyVariants.push(variant);
            break;
          default:
            console.error('unexpected variant type', variant);
            break;
        }
      }
    }
    return html`
      ${unexpectedVariants.length === 0 ? html`
      <div class="list-entry">No unexpected test results.</div>
      <hr class="divider">
      ` : ''}
      ${this.renderVariants(unexpectedVariants)}
      ${this.renderVariants(flakyVariants)}
      ${this.renderVariants(exoneratedVariants)}
      ${this.renderVariants(expectedVariants)}
    `;
  }

  private renderVariants(variants: ReadonlyVariant[]) {
    return html`
      ${repeat(
        variants.map((v, i, variants) => [variants[i-1], v, variants[i+1]] as [ReadonlyVariant | undefined, ReadonlyVariant, ReadonlyVariant | undefined]),
        ([_, v]) => `${v.testId} ${v.variantKey}`,
        ([prev, v, next]) => html`
        <tr-variant-entry
          .variant=${v}
          .prevTestId=${prev?.testId ?? ''}
          .prevVariant=${prev?.testId === v.testId ? prev : null}
          .expanded=${this.hasSingleVariant}
          .displayVariantId=${prev?.testId === v.testId || next?.testId === v.testId}
        ></tr-variant-entry>
      `)}
      ${variants.length !== 0 ? html`<hr class="divider">` : ''}
    `;
  }

  protected render() {
    const state = this.pageState;

    return html`
      <div id="header">
        <tr-test-filter
          .onFilterChanged=${(filter: TestFilter) => {
            this.pageState.showExonerated = filter.showExonerated;
            this.pageState.showExpected = filter.showExpected;
            this.pageState.showFlaky = filter.showFlaky;
          }}
        >
        </tr-test-filter>
        <mwc-button
          class="action-button"
          dense unelevated
          @click=${() => this.toggleAllVariants(true)}
        >Expand All</mwc-button>
        <mwc-button
          class="action-button"
          dense unelevated
          @click=${() => this.toggleAllVariants(false)}
        >Collapse All</mwc-button>
      </div>
      <div id="main">
        <tr-left-panel>
          <tr-test-nav-tree
            .testLoader=${state.testLoader}
            .onSelectedNodeChanged=${(node: TestNode) => state.selectedNode = node}
          ></tr-test-nav-tree>
        </tr-left-panel>
        <div id="test-result-view">
          ${this.renderAllVariants()}
          <div class="list-entry">
            <span>Showing ${state.selectedNode.testCount} tests.</span>
            <span
              id="load"
              style=${styleMap({'display': state.testLoader.done ? 'none' : ''})}
            >
              <span
                id="load-more"
                style=${styleMap({'display': state.testLoader.isLoading ? 'none' : ''})}
                @click=${this.loadNextPage}
              >
                Load More
              </span>
              <span
                style=${styleMap({'display': state.testLoader.isLoading ? '' : 'none'})}
              >
                Loading <tr-dot-spinner></tr-dot-spinner>
              </span>
              <mwc-icon id="load-info" title="Newly loaded entries might be inserted into the list.">info</mwc-icon>
            </span>
          </div>
        </div>
      </div>
    `;
  }

  static styles = css`
    :host {
      display: grid;
      grid-template-rows: auto 1fr;
      overflow-y: hidden;
    }

    #header {
      display: grid;
      grid-template-columns: 1fr auto auto;
    }

    .action-button {
      margin: 0 5px;
      --mdc-theme-primary: rgb(0, 123, 255);
    }

    #main {
      display: flex;
      border-top: 1px solid #DDDDDD;
      overflow-y: hidden;
    }
    #test-result-view {
      flex: 1;
      overflow-y: auto;
      padding-top: 5px;
    }
    #test-result-view>* {
      margin-bottom: 2px;
    }

    .divider {
      border: none;
      border-top: 1px solid #DDDDDD;
    }

    tr-test-nav-tree {
      overflow: hidden;
    }

    .list-entry {
      margin-left: 5px;
      margin-top: 5px;
    }
    #load {
      color: blue;
    }
    #load-more {
      color: blue;
      cursor: pointer;
    }
    #load-info {
      color: #212121;
      --mdc-icon-size: 1.2em;
      vertical-align: bottom;
    }
  `;
}

customElement('tr-test-results-tab')(
  consumeContext('pageState')(
      TestResultsTabElement,
  ),
);
