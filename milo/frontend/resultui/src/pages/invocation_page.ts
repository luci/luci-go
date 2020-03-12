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
import { BeforeEnterObserver, PreventAndRedirectCommands, Router, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { action, computed, observable, when } from 'mobx';
import moment from 'moment';

import '../components/invocation_details';
import '../components/page_header';
import '../components/status_bar';
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
export class InvocationPageElement extends MobxLitElement implements
    BeforeEnterObserver {
  @observable.ref accessToken = '';
  @observable.ref invocationName = '';

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

  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    this.refreshAccessToken();
    const invocationName = location.params['invocation_name'];
    if (typeof invocationName !== 'string') {
      return cmd.redirect('/not-found');
    }
    this.invocationName = invocationName;
    return;
  }

  private disposer: () => void = () => {};
  connectedCallback() {
    super.connectedCallback();
    window.addEventListener('user-update', this.refreshAccessToken);
    this.refreshAccessToken();
    this.disposer = when(
      () => this.invocationReq.get().tag === 'err',
      () => Router.go('/error'),
    );
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('user-update', this.refreshAccessToken);
    this.disposer();
  }
  @action
  private refreshAccessToken = () => {
    // Awaiting on authInstance to load may block the loading of authInstance,
    // creating a deadlock. Use synced call instead.
    this.accessToken = signin.getAuthInstanceSync()
                           ?.currentUser.get()
                           .getAuthResponse()
                           .access_token ||
        '';
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
          <!-- TODO(weiweilin): use <chops-timestamp> -->
          at ${moment(this.invocation.finalizeTime).format('YYYY-MM-DD HH:mm:ss')}
        `;
    }

    return html`
      <i>${INVOCATION_STATE_DISPLAY_MAP[this.invocation.state]}</i>
      <!-- TODO(weiweilin): use <chops-timestamp> -->
      since ${moment(this.invocation.createTime).format('YYYY-MM-DD HH:mm:ss')}
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
    `;
  }

  static styles = css`
    #test-invocation-summary {
      font-size: 16px;
      line-spacing: 0.15px;
      padding: 5px;
    }
  `;

}
