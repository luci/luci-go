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

import { css, customElement, html, LitElement, property } from 'lit-element';

import { ANONYMOUS_IDENTITY } from '../services/milo_internal';

/**
 * `milo-signin` is a web component that manages signing into services using
 * client-side OAuth via gapi.auth2. milo-signin visually indicates whether the
 * user is signed in using either an icon or the user's profile picture. The
 * signin or signout flow is initiated when the user clicks on this component.
 *
 * @param gAuth: gapi.auth2.GoogleAuth You must provide a GoogleAuth instance.
 * Updating gAuth is noop.
 * @event user-update: emits a UserUpdateEvent when user's profile is updated.
 */
@customElement('milo-signin')
export class SignInElement extends LitElement {
  @property() private identity?: string;
  @property() private email?: string;
  @property() private picture?: string;

  protected render() {
    if (!this.identity || this.identity === ANONYMOUS_IDENTITY) {
      return html`<a target="_blank" href="/auth/openid/login">Login</a>`;
    }
    return html`
      ${this.picture ? html`<img src=${this.picture} />` : ''}
      <div>${this.email} | <a target="_blank" href="/auth/openid/logout">Logout</a></div>
    `;
  }

  static styles = css`
    :host {
      display: inline-block;
      height: 32px;
      line-height: 32px;
    }
    a {
      color: var(--default-text-color);
    }
    img {
      margin: 2px 3px;
      height: 28px;
      width: 28px;
      border-radius: 6px;
      overflow: hidden;
    }
    div {
      display: inline-block;
      height: 32px;
      vertical-align: top;
    }
  `;
}
