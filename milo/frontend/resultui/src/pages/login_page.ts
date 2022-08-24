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
import { makeObservable, observable, when } from 'mobx';

import { changeUserState } from '../components/signin';
import { consumer } from '../libs/context';
import { ANONYMOUS_IDENTITY } from '../services/milo_internal';
import { consumeStore, StoreInstance } from '../store';
import commonStyle from '../styles/common_style.css';

/**
 * Prompts the user to login.
 * Once logged in, redirects to
 *   - URL specified in 'redirect' search param, or
 *   - URL it is redirected from, or
 *   - root.
 * in that order.
 */
@customElement('milo-login-page')
@consumer
export class LoginPageElement extends MobxLitElement implements BeforeEnterObserver {
  @observable.ref
  @consumeStore()
  store!: StoreInstance;

  private redirectUri = '';

  constructor() {
    super();
    makeObservable(this);
  }

  onBeforeEnter(location: RouterLocation) {
    const redirect = new URLSearchParams(location.search).get('redirect');
    this.redirectUri = redirect || location.redirectFrom || '/';
  }

  connectedCallback() {
    super.connectedCallback();
    when(
      () => ![undefined, ANONYMOUS_IDENTITY].includes(this.store.userIdentity),
      () => Router.go(this.redirectUri)
    );
  }

  protected render() {
    return html`<div id="sign-in-message">
      You must <a @click=${() => changeUserState(true)}>sign in</a> to see anything useful.
    </div>`;
  }

  static styles = [
    commonStyle,
    css`
      #sign-in-message {
        margin: 8px 16px;
      }
      a {
        text-decoration: underline;
        cursor: pointer;
      }
    `,
  ];
}
