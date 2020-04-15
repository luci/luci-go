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
import '@material/mwc-icon';
import { BeforeEnterObserver, PreventAndRedirectCommands, Router, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { classMap } from 'lit-html/directives/class-map';
import { repeat } from 'lit-html/directives/repeat';
import { styleMap } from 'lit-html/directives/style-map';
import { computed, observable, reaction, when } from 'mobx';

import '../components/invocation_details';
import '../components/page_header';
import '../components/status_bar';
import '../components/test-entry';
import '../components/test_nav_tree';
import { contextConsumer } from '../context';
import { AppState } from '../context/app_state';
import { streamTestExonerations, streamTestResults, streamTests, TestLoader } from '../models/test_loader';
import { ReadonlyTest, TestNode } from '../models/test_node';
import { Invocation, InvocationState } from '../services/resultdb';

const INVOCATION_STATE_DISPLAY_MAP = {
  [InvocationState.Unspecified]: 'unspecified',
  [InvocationState.Active]: 'active',
  [InvocationState.Finalizing]: 'finalizing',
  [InvocationState.Finalized]: 'finalized',
};

/**
 * Main test results page.
 * Reads invocation_name from URL params.
 * If not logged in, redirects to '/login?redirect=${current_url}'.
 * If invocation_name not provided, redirects to '/not-found'.
 * Otherwise, shows results for the invocation.
 */
export class InvocationPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref invocationName = '';
  @observable.ref leftPanelExpanded = false;
  @observable.ref pageLength = 100;

  @observable.ref appState!: AppState;

  @computed
  private get invocationReq(): ObservablePromise<Invocation> {
    if (!this.appState?.resultDb) {
      return observable.box({tag: 'loading', v: null}, {deep: false});
    }
    return this.appState.resultDb
      .getInvocation({name: this.invocationName})
      .toObservable();
  }

  @computed
  private get invocation(): Invocation | null {
    const req = this.invocationReq.get();
    if (req.tag !== 'ok') {
      return null;
    }
    return req.v;
  }

  @computed
  private get testIter(): AsyncIterableIterator<ReadonlyTest> {
    if (!this.appState?.resultDb) {
      return (async function*() {})();
    }
    return streamTests(
      streamTestResults({invocations: [this.invocationName]}, this.appState.resultDb),
      streamTestExonerations({invocations: [this.invocationName]}, this.appState.resultDb),
    );
  }

  // testLoader is used by non-observers, keep it alive.
  @computed({keepAlive: true})
  private get testLoader() {
    const ret = new TestLoader(TestNode.newRoot(), this.testIter);
    return ret;
  }

  @computed
  private get selectedTests(): readonly ReadonlyTest[] {
    // TODO(weiweilin): implement this.selectedTests()
    return this.testLoader.node.allTests;
  }

  @computed
  private get rootName(): string {
    return this.testLoader.node.name;
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const invocationName = location.params['invocation_name'];
    if (typeof invocationName !== 'string') {
      return cmd.redirect('/not-found');
    }
    this.invocationName = invocationName;
    return;
  }

  private disposers: Array<() => void> = [];
  connectedCallback() {
    super.connectedCallback();
    this.disposers.push(when(
      () => this.invocationReq.get().tag === 'err',
      () => Router.go('/error'),
    ));
    // Load the first batch whenever this.testLoader is updated.
    this.disposers.push(reaction(
      () => this.testLoader,
      () => {
        this.testLoader.loadMore();
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

  private renderInvocationState() {
    if (!this.invocation) {
      return null;
    }
    if (this.invocation.finalizeTime) {
        return html`
          <i>${INVOCATION_STATE_DISPLAY_MAP[this.invocation.state]}</i>
          at ${new Date(this.invocation.finalizeTime).toLocaleString()}
        `;
    }

    return html`
      <i>${INVOCATION_STATE_DISPLAY_MAP[this.invocation.state]}</i>
      since ${new Date(this.invocation.createTime).toLocaleString()}
    `;
  }

  protected render() {
    return html`
      <div id="test-invocation-summary">
        <div id="test-invocation-id">
          <span id="test-invocation-id-label">Invocation ID </span>
          <span>${this.invocationName.slice('invocations/'.length)}</span>
        </div>
        <div id="test-invocation-state">${this.renderInvocationState()}</div>
      </div>
      <tr-status-bar
        .components=${[{color: '#007bff', weight: 1}]}
        .loading=${this.invocationReq.get().tag === 'loading'}
      ></tr-status-bar>
      ${!this.invocation ? null : html`
      <tr-invocation-details .invocation=${this.invocation}></tr-invocation-details>
      `}
      <div id="main" class=${classMap({'show-left-panel': this.leftPanelExpanded})}>
        <div
          id="left-panel"
          style=${styleMap({display: this.leftPanelExpanded ? '' : 'none'})}
        >
          <tr-test-nav-tree .testLoader=${this.testLoader}></tr-test-nav-tree>
        </div>
        <div id="test-result-view">
          <div id="test-result-header">
            <div id="menu-button" @click=${() => this.leftPanelExpanded = !this.leftPanelExpanded}>
              <mwc-icon id="menu-icon">menu</mwc-icon>
            </div>
            <span id="root-name" title="common test ID prefix">${this.rootName}</span>
          </div>
          <div id="test-result-content">
            ${repeat(this.selectedTests.slice(0, this.pageLength), (t) => t.id, (t, i) => html`
            <tr-test-entry
              .test=${t}
              .rootName=${this.rootName}
              .prevTestId=${(this.selectedTests[i-1]?.id || this.rootName)}
              .expanded=${this.selectedTests.length === 1}
            ></tr-test-entry>
            `)}
            <div id="list-tail">
              <span>Showing ${Math.min(this.selectedTests.length, this.pageLength)}/${this.selectedTests.length} tests.</span>
              <span
                id="load-more"
                style=${styleMap({'display': this.pageLength >= this.selectedTests.length ? 'none' : ''})}
                @click=${() => {
                  // TODO(weiweilin): trigger this.testLoader.loadMore() when
                  // needed.
                  this.pageLength += 100;
                }}
              >
                Load More
              </span>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  static styles = css`
    :host {
      height: calc(100% - var(--header-height));
      display: flex;
      flex-direction: column;
      overflow-y: hidden;
    }

    #test-invocation-summary {
      background-color: rgb(248, 249, 250);
      padding: 6px 16px;
      font-family: "Google sans";
      font-size: 14px;
      display: flex;
    }

    #test-invocation-id {
      flex: 0 auto;
    }

    #test-invocation-id-label {
      color: rgb(95, 99, 104);
    }

    #test-invocation-state {
      margin-left: auto;
      flex: 0 auto;
    }

    #main {
      display: flex;
      flex: 1;
      border-top: 2px solid #DDDDDD;
      overflow-y: hidden;
    }
    #left-panel {
      overflow-y: hidden;
      border-right: 2px solid #DDDDDD;
      width: 400px;
      resize: horizontal;
    }
    #test-result-view {
      flex: 1;
      display: flex;
      flex-direction: column;
      overflow-y: hidden;
    }
    #test-result-header {
      width: 100%;
      height: 32px;
      background: #DDDDDD;
    }
    #menu-button {
      display: inline-table;
      height: 100%;
      cursor: pointer;
    }
    #menu-icon {
      display: table-cell;
      vertical-align: middle;
    }

    #test-result-content {
      overflow-y: auto;
    }
    #root-name {
      font-size: 16px;
      letter-spacing: 0.15px;
      vertical-align: middle;
      display: inline-table;
      height: 100%;
      margin-left: 5px;
    }

    #list-tail {
      margin: 5px;
    }
    #load-more {
      color: blue;
      cursor: pointer;
    }
  `;
}

customElement('tr-invocation-page')(
  contextConsumer('appState')(InvocationPageElement),
);
