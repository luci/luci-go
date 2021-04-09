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

import '@material/mwc-icon';
import { BeforeEnterObserver, PreventAndRedirectCommands, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { autorun, computed, observable, reaction } from 'mobx';
import { REJECTED } from 'mobx-utils';

import '../../components/status_bar';
import '../../components/tab_bar';
import './invocation_details_tab';
import { MiloBaseElement } from '../../components/milo_base';
import { TabDef } from '../../components/tab_bar';
import { AppState, consumeAppState } from '../../context/app_state';
import { InvocationState, provideInvocationState } from '../../context/invocation_state';
import { consumeConfigsStore, UserConfigsStore } from '../../context/user_configs';
import { INVOCATION_STATE_DISPLAY_MAP } from '../../libs/constants';
import { NOT_FOUND_URL, router } from '../../routes';

/**
 * Main test results page.
 * Reads invocation_id from URL params.
 * If not logged in, redirects to '/login?redirect=${current_url}'.
 * If invocation_id not provided, redirects to '/not-found'.
 * Otherwise, shows results for the invocation.
 */
@customElement('milo-invocation-page')
@provideInvocationState
@consumeConfigsStore
@consumeAppState
export class InvocationPageElement extends MiloBaseElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  @observable.ref configsStore!: UserConfigsStore;
  @observable.ref invocationState!: InvocationState;

  private invocationId = '';
  onBeforeEnter(location: RouterLocation, cmd: PreventAndRedirectCommands) {
    const invocationId = location.params['invocation_id'];
    if (typeof invocationId !== 'string') {
      return cmd.redirect(NOT_FOUND_URL);
    }
    this.invocationId = invocationId;
    return;
  }

  connectedCallback() {
    super.connectedCallback();

    this.addDisposer(
      reaction(
        () => [this.appState],
        ([appState]) => {
          this.invocationState?.dispose();
          this.invocationState = new InvocationState(appState);
          this.invocationState.invocationId = this.invocationId;

          // Emulate @property() update.
          this.updated(new Map([['invocationState', this.invocationState]]));
        },
        { fireImmediately: true }
      )
    );
    this.addDisposer(() => this.invocationState.dispose());

    this.addDisposer(
      autorun(() => {
        if (this.invocationState.invocation$.state !== REJECTED) {
          return;
        }
        this.dispatchEvent(
          new ErrorEvent('error', {
            message: this.invocationState.invocation$.value.toString(),
            composed: true,
            bubbles: true,
          })
        );
      })
    );
    document.title = `inv: ${this.invocationId}`;
  }

  private renderInvocationState() {
    const invocation = this.invocationState.invocation;
    if (!invocation) {
      return null;
    }
    if (invocation.finalizeTime) {
      return html`
        <i>${INVOCATION_STATE_DISPLAY_MAP[invocation.state]}</i>
        at ${new Date(invocation.finalizeTime).toLocaleString()}
      `;
    }

    return html`
      <i>${INVOCATION_STATE_DISPLAY_MAP[invocation.state]}</i>
      since ${new Date(invocation.createTime).toLocaleString()}
    `;
  }

  @computed get tabDefs(): TabDef[] {
    return [
      {
        id: 'test-results',
        label: 'Test Results',
        href: router.urlForName(
          this.configsStore.userConfigs.testResults.newLayout
            ? 'invocation-test-results-new'
            : 'invocation-test-results',
          { invocation_id: this.invocationState.invocationId! }
        ),
      },
      {
        id: 'invocation-details',
        label: 'Invocation Details',
        href: router.urlForName('invocation-details', { invocation_id: this.invocationState.invocationId! }),
      },
    ];
  }

  protected render() {
    if (this.invocationState.invocationId === '') {
      return html``;
    }

    return html`
      <div id="test-invocation-summary">
        <div id="test-invocation-id">
          <span id="test-invocation-id-label">Invocation ID </span>
          <span>${this.invocationState.invocationId}</span>
        </div>
        <div id="test-invocation-state">${this.renderInvocationState()}</div>
      </div>
      <milo-status-bar
        .components=${[{ color: 'var(--active-color)', weight: 1 }]}
        .loading=${this.invocationState.invocation$.state === 'pending'}
      ></milo-status-bar>
      <milo-tab-bar .tabs=${this.tabDefs} .selectedTabId=${this.appState.selectedTabId}></milo-tab-bar>
      <slot></slot>
    `;
  }

  static styles = css`
    :host {
      height: calc(100vh - var(--header-height));
      display: grid;
      grid-template-rows: repeat(3, auto) 1fr;
    }

    #test-invocation-summary {
      background-color: var(--block-background-color));
      padding: 6px 16px;
      font-family: "Google Sans", "Helvetica Neue", sans-serif;
      font-size: 14px;
      display: flex;
    }

    milo-tab-bar {
      margin: 0 10px;
      padding-top: 10px;
    }

    #test-invocation-id {
      flex: 0 auto;
    }

    #test-invocation-id-label {
      color: var(--light-text-color);
    }

    #test-invocation-state {
      margin-left: auto;
      flex: 0 auto;
    }
  `;
}
