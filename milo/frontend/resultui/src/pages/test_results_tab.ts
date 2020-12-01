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

import '../components/hotkey';
import '../components/left_panel';
import '../components/test_filter';
import '../components/test_nav_tree';
import '../components/variant_entry';
import { VariantEntryElement } from '../components/variant_entry';
import { AppState, consumeAppState } from '../context/app_state/app_state';
import { consumeConfigsStore, UserConfigsStore } from '../context/app_state/user_configs';
import { consumeInvocationState, InvocationState } from '../context/invocation_state/invocation_state';
import { ReadonlyVariant, TestNode, VariantStatus } from '../models/test_node';

/**
 * Display a list of test results.
 */
export class TestResultsTabElement extends MobxLitElement {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref invocationState!: InvocationState;

  private disposers: Array<() => void> = [];
  private async loadNextPage() {
    try {
      await this.invocationState.testLoader.loadNextPage();
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
    return this.invocationState.selectedNode.testCount === 1 && [...this.invocationState.selectedNode.tests()].length === 1;
  }

  @observable.ref private allVariantsWereExpanded = false;
  private toggleAllVariants(expand: boolean) {
    this.allVariantsWereExpanded = expand;
    this.shadowRoot!.querySelectorAll<VariantEntryElement>('milo-variant-entry')
      .forEach((e) => e.expanded = expand);
  }
  private readonly toggleAllVariantsByHotkey = () => this.toggleAllVariants(!this.allVariantsWereExpanded);

  connectedCallback() {
    super.connectedCallback();
    this.appState.selectedTabId = 'test-results';

    // When a new test loader is received, load the first page and reset the
    // selected node.
    this.disposers.push(reaction(
      () => this.invocationState.testLoader,
      (testLoader) => {
        this.loadNextPage();
        this.invocationState.selectedNode = testLoader.node;
      },
      {fireImmediately: true},
    ));

    // Update filters to match the querystring without saving them.
    const searchParams = new URLSearchParams(window.location.search);
    if (searchParams.has('expected')) {
      this.configsStore.userConfigs.tests.showExpectedVariant = searchParams.get('expected') === 'true';
    }
    if (searchParams.has('exonerated')) {
      this.configsStore.userConfigs.tests.showExoneratedVariant = searchParams.get('exonerated') === 'true';
    }
    if (searchParams.has('flaky')) {
      this.configsStore.userConfigs.tests.showFlakyVariant = searchParams.get('flaky') === 'true';
    }
    // Update the querystring when filters are updated.
    this.disposers.push(reaction(
      () => {
        const newSearchParams = new URLSearchParams({
          expected: String(this.configsStore.userConfigs.tests.showExpectedVariant),
          exonerated: String(this.configsStore.userConfigs.tests.showExoneratedVariant),
          flaky: String(this.configsStore.userConfigs.tests.showFlakyVariant),
        });
        return newSearchParams.toString();
      },
      (newQueryStr) => {
        const newUrl = `${window.location.protocol}//${window.location.host}${window.location.pathname}?${newQueryStr}`;
        window.history.pushState({path: newUrl}, '', newUrl);
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
    for (const test of this.invocationState.selectedNode.tests()) {
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
      ` : html ``}
      ${this.renderIntegrationHint()}
      ${this.renderLoadMore()}
      ${this.renderVariants(unexpectedVariants)}
      ${this.renderVariants(flakyVariants)}
      ${this.renderVariants(exoneratedVariants)}
      ${this.renderVariants(expectedVariants)}
    `;
  }

  private renderIntegrationHint() {
    const state = this.invocationState;
    return this.configsStore.userConfigs.hints.showTestResultsHint && !state.testLoader.isLoading? html `
      <div class="list-entry">
        <p>
        Don't see results of your test framework here?
        This might be because they are not integrated with ResultDB yet.
        Please ask <a href="mailto: luci-eng@google.com" target="_blank">luci-eng@</a> for help.
        </p>
        Known issues:
        <ul id="knownissues">
          <li>Test result tab is currently slow: <a href="https://crbug.com/1114935">crbug.com/1114935</a>.</li>
          <li>Sometimes no test failures are displayed at first load: <a href="https://crbug.com/1111683">crbug.com/1111683</a>. You may need to click on "Load More" link.</li>
        </ul>
        <span
          id="hide-hint"
          @click=${() => {
            this.configsStore.userConfigs.hints.showTestResultsHint = false;
            this.configsStore.save();
          }}
        >Don't show again</span>
      </div>
      <hr class="divider">
    `: html ``;
  }

  private renderVariants(variants: ReadonlyVariant[]) {
    return html`
      ${repeat(
        variants.map((v, i, variants) => [variants[i-1], v, variants[i+1]] as [ReadonlyVariant | undefined, ReadonlyVariant, ReadonlyVariant | undefined]),
        ([_, v]) => `${v.testId} ${v.variantHash}`,
        ([prev, v, next]) => html`
        <milo-variant-entry
          .variant=${v}
          .prevTestId=${prev?.testId ?? ''}
          .prevVariant=${prev?.testId === v.testId ? prev : null}
          .expanded=${this.hasSingleVariant}
          .displayVariantId=${prev?.testId === v.testId || next?.testId === v.testId}
        ></milo-variant-entry>
      `)}
      ${variants.length !== 0 ? html`<hr class="divider">` : ''}
    `;
  }

  private renderLoadMore() {
    const state = this.invocationState;
    return html`
      <div id="loadmore" class="list-entry">
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
            Loading <milo-dot-spinner></milo-dot-spinner>
          </span>
          <mwc-icon class="inline-icon" title="Newly loaded entries might be inserted into the list.">info</mwc-icon>
        </span>
      </div>
    `;
  }

  private renderMain() {
    const state = this.invocationState;

    if (state.initialized && !state.invocationId) {
      return html`
        <div id="no-invocation">
          No associated invocation.<br>
          You need to integrate with ResultDB to see the test results.<br>
          See <a href="http://go/resultdb" target="_blank">go/resultdb</a>
          or ask <a href="mailto: luci-eng@google.com" target="_blank">luci-eng@</a> for help.
        </div>
      `;
    }

    return html`
      <milo-left-panel>
        <milo-test-nav-tree
          .testLoader=${state.testLoader}
          .onSelectedNodeChanged=${(node: TestNode) => state.selectedNode = node}
        ></milo-test-nav-tree>
      </milo-left-panel>
      <milo-hotkey
        key="space,shift+space,up,down,pageup,pagedown"
        style="display: none;"
        .handler=${() => this.shadowRoot!.getElementById('test-result-view')!.focus()}
      ></milo-hotkey>
      <div id="test-result-view" tabindex="-1">
        ${this.renderAllVariants()}
      </div>
    `;
  }

  protected render() {
    return html`
      <div id="header">
        <milo-test-filter></milo-test-filter>
        <milo-hotkey key="x" .handler=${this.toggleAllVariantsByHotkey} title="press x to expand/collapse all entries">
          <mwc-button
            dense unelevated
            @click=${() => this.toggleAllVariants(true)}
          >Expand All</mwc-button>
          <mwc-button
            dense unelevated
            @click=${() => this.toggleAllVariants(false)}
          >Collapse All</mwc-button>
        </milo-hotkey>
      </div>
      <div id="main">${this.renderMain()}</div>
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
      grid-template-columns: 1fr auto;
      grid-gap: 5px;
      height: 28px;
      padding: 5px 10px 3px 10px;
    }

    #loadmore {
      padding-bottom: 5px;
    }

    milo-test-filter {
      margin: 5px;
      margin-bottom: 0px;
    }

    #main {
      display: flex;
      border-top: 1px solid var(--divider-color);
      overflow-y: hidden;
    }
    #no-invocation {
      padding: 10px;
    }
    #test-result-view {
      flex: 1;
      overflow-y: auto;
      padding-top: 5px;
      outline: none;
    }
    #test-result-view>* {
      margin-bottom: 2px;
    }

    .divider {
      border: none;
      border-top: 1px solid var(--divider-color);
    }

    milo-test-nav-tree {
      overflow: hidden;
    }

    .list-entry {
      margin-left: 5px;
      margin-top: 5px;
    }
    #load {
      color: var(--active-text-color);
    }
    #load-more {
      color: var(--active-text-color);
      cursor: pointer;
    }
    .inline-icon {
      color: var(--default-text-color);
      --mdc-icon-size: 1.2em;
      vertical-align: bottom;
    }
    #hide-hint {
      color: var(--active-text-color);
      cursor: pointer;
    }
    #knownissues{
      margin-top: 3px;
      margin-bottom: 3px;
    }
  `;
}

customElement('milo-test-results-tab')(
  consumeInvocationState(
    consumeConfigsStore(
      consumeAppState(TestResultsTabElement),
    ),
  ),
);
