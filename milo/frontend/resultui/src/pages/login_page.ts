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
import { BeforeEnterObserver, Router, RouterLocation } from '@vaadin/router';
import { css, customElement, html } from 'lit-element';
import { observable, when } from 'mobx';
import { AppState, consumeAppState } from '../context/app_state';

/**
 * Prompts the user to login.
 * Once logged in, redirects to
 *   - URL specified in 'redirect' search param, or
 *   - URL it is redirected from, or
 *   - root.
 * in that order.
 */
@customElement('milo-login-page')
@consumeAppState
export class LoginPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref appState!: AppState;
  private redirectUri = '';

  onBeforeEnter(location: RouterLocation) {
    const redirect = new URLSearchParams(location.search).get('redirect');
    this.redirectUri = redirect || location.redirectFrom || '/';
  }

  connectedCallback() {
    super.connectedCallback();
    when(
      () => !!this.appState.accessToken,
      () => Router.go(this.redirectUri)
    );
  }

  protected render() {
    if (!this.appState.gAuth) {
      return html`
        <div id="sign-in-message">
          You must sign in to see anything useful, but Google signin failed to initialize.<br />
          Please ensure that cookies from <i>[*.]accounts.google.com</i> are not blocked.
          <div></div>
        </div>
      `;
    }
    return html`
      <div id="sign-in-message">
        You must
        <span id="sign-in-link" @click=${() => this.appState.gAuth?.signIn()}>sign in</span>
        to see anything useful.
        <div></div>
      </div>
    `;
  }

  static styles = css`
    #sign-in-message {
      margin: 8px 16px;
    }

    #sign-in-link {
      cursor: pointer;
      text-decoration: underline;
    }
  `;
}
