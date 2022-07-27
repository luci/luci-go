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
import { BroadcastChannel } from 'broadcast-channel';
import { css, customElement, html } from 'lit-element';
import { makeObservable, observable } from 'mobx';

import { ANONYMOUS_IDENTITY } from '../services/milo_internal';

/**
 * Signs in/out the user.
 *
 * @param signIn pass true to sign in, false to sign out.
 */
export function changeUserState(signIn: boolean) {
  const channelId = 'auth-channel-' + Math.random();
  const redirectUrl = `/ui/auth-callback/${channelId}`;
  const target = window.open(
    `/auth/openid/${signIn ? 'login' : 'logout'}?${new URLSearchParams({ r: redirectUrl })}`,
    '_blank'
  );
  if (!target) {
    return;
  }

  const channel = new BroadcastChannel(channelId);

  // Close the channel in 1hr to prevent memory leak in case the target never
  // sent the message.
  const timeout = window.setTimeout(() => channel.close(), 3600000);

  channel.addEventListener('message', () => {
    window.clearTimeout(timeout);
    channel.close();
    target.close();
  });
}

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
export class SignInElement extends MobxLitElement {
  @observable.ref private identity?: string;
  @observable.ref private email?: string;
  @observable.ref private picture?: string;

  constructor() {
    super();
    makeObservable(this);
  }

  protected render() {
    if (!this.identity || this.identity === ANONYMOUS_IDENTITY) {
      return html`<a @click=${() => changeUserState(true)}>Login</a>`;
    }
    return html`
      ${this.picture ? html`<img src=${this.picture} />` : ''}
      <div>${this.email} | <a @click=${() => changeUserState(false)}>Logout</a></div>
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
      text-decoration: underline;
      cursor: pointer;
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
