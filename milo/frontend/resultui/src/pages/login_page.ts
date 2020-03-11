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

import '../components/page_header';

import {MobxLitElement} from '@adobe/lit-mobx';
import * as signin from '@chopsui/chops-signin';
import {BeforeEnterObserver, Router, RouterLocation} from '@vaadin/router';
import {customElement, html} from 'lit-element';

/**
 * Prompts the user to login.
 * Once logged in, redirects to
 *   - URL specified in 'redirect' search param, or
 *   - URL it is redirected from, or
 *   - root.
 * in that order.
 */
@customElement('tr-login-page')
export class LoginPageElement extends MobxLitElement implements
    BeforeEnterObserver {
  redirectUri = '';

  protected render() {
    return html`
      <tr-page-header></tr-page-header>
      <div>You must sign in to see anything useful.<div>
    `;
  }

  onBeforeEnter(location: RouterLocation) {
    const redirect = new URLSearchParams(location.search).get('redirect');
    this.redirectUri = redirect || location.redirectFrom || '/';
  }

  connectedCallback() {
    super.connectedCallback();
    window.addEventListener('user-update', this.onUserUpdate);
    this.onUserUpdate();
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('user-update', this.onUserUpdate);
  }
  private onUserUpdate = () => {
    if (signin.getAuthorizationHeadersSync()?.Authorization) {
      Router.go(this.redirectUri);
    }
  }
}
