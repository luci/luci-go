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

import * as signin from '@chopsui/chops-signin';
import { BeforeEnterObserver, Router } from '@vaadin/router';
import { customElement, html, LitElement } from 'lit-element';
import { action } from 'mobx';

import { router } from '../../routes';
import { consumeResultDbHost } from '../config_provider';
import { AppState, provideAppState } from './app_state';

/**
 * Provides appState to be shared across the app.
 * Listens to user-update event and updates appState.accessToken accordingly.
 */
export class AppStateProviderElement extends LitElement implements BeforeEnterObserver {
  appState = new AppState();
  resultDbHost = '';

  onBeforeEnter() {
    this.refreshAccessToken();
  }
  connectedCallback() {
    super.connectedCallback();
    this.appState.resultDbHost = this.resultDbHost;
    window.addEventListener('user-update', this.refreshAccessToken);
    this.refreshAccessToken();
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('user-update', this.refreshAccessToken);
  }

  protected render() {
    return html`
      <slot></slot>
    `;
  }

  @action
  private refreshAccessToken = () => {
    // Awaiting on authInstance to load may block the loading of authInstance,
    // creating a deadlock. Use synced call instead.
    const authInstance = signin.getAuthInstanceSync();
    if (!authInstance) {
      return;
    }

    this.appState.accessToken = authInstance
      .currentUser.get()
      .getAuthResponse()
      .access_token || '';

    if (!this.appState.accessToken) {
      const searchParams = new URLSearchParams();
      searchParams.set('redirect', window.location.href);
      return Router.go(`${router.urlForName('login')}?${searchParams}`);
    }
    return;
  }
}

customElement('tr-app-state-provider')(
  provideAppState(
    consumeResultDbHost(
      AppStateProviderElement,
    ),
  ),
);
