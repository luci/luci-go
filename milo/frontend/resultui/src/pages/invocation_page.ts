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
import * as signin from '@chopsui/chops-signin';
import '@material/mwc-icon';
import { BeforeEnterObserver, PreventAndRedirectCommands, Router, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { classMap } from 'lit-html/directives/class-map';
import { styleMap } from 'lit-html/directives/style-map';
import { action, computed, observable, reaction, when } from 'mobx';

import '../components/invocation_details';
import '../components/page_header';
import '../components/status_bar';
import '../components/test_nav_tree';
import { streamTestExonerations, streamTestResults, streamTests, TestLoader } from '../models/test_loader';
import { ReadonlyTest, TestNode } from '../models/test_node';
import { Invocation, InvocationState, ResultDb } from '../services/resultdb';

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
@customElement('tr-invocation-page')
export class InvocationPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref accessToken = '';
  @observable.ref invocationName = '';
  @observable.ref leftPanelExpanded = false;

  @computed
  private get resultDb(): ResultDb | null {
    if (!this.accessToken) {
      return null;
    }
    // TODO(weiweilin): set the host dynamically (from a config file?).
    return new ResultDb('staging.results.api.cr.dev', this.accessToken);
  }

  @computed
  private get invocationReq(): ObservablePromise<Invocation> {
    if (!this.resultDb) {
      return observable.box({tag: 'loading', v: null}, {deep: false});
    }
    return this.resultDb
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
    if (!this.resultDb) {
      return (async function*() {})();
    }
    return streamTests(
      streamTestResults({invocations: [this.invocationName]}, this.resultDb),
      streamTestExonerations({invocations: [this.invocationName]}, this.resultDb),
    );
  }

  // testLoader is used by non-observers, keep it alive.
  @computed({keepAlive: true})
  private get testLoader() {
    const ret = new TestLoader(TestNode.newRoot(), this.testIter);
    return ret;
  }

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    this.refreshAccessToken();
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
    window.addEventListener('user-update', this.refreshAccessToken);
    this.refreshAccessToken();
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
    window.removeEventListener('user-update', this.refreshAccessToken);
    for (const disposer of this.disposers) {
      disposer();
    }
  }
  @action
  private refreshAccessToken = () => {
    // Awaiting on authInstance to load may block the loading of authInstance,
    // creating a deadlock. Use synced call instead.
    this.accessToken = signin
      .getAuthInstanceSync()
      ?.currentUser.get()
      .getAuthResponse()
      .access_token || '';
    if (!this.accessToken) {
      const searchParams = new URLSearchParams();
      searchParams.set('redirect', window.location.href);
      return Router.go(`/login?${searchParams}`);
    }
    return;
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
      <tr-page-header></tr-page-header>
      <div id="test-invocation-summary">
          <span>${this.invocationName.slice('invocations/'.length)}</span>
          <span style="float: right">${this.renderInvocationState()}</span>
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
          </div>
        </div>
      </div>
    `;
  }

  static styles = css`
    :host {
      height: 100%;
      display: flex;
      flex-direction: column;
      overflow-y: hidden;
    }

    #test-invocation-summary {
      font-size: 16px;
      line-spacing: 0.15px;
      padding: 5px;
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
  `;
}
