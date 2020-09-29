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

import { css, customElement, html, LitElement, property, svg } from 'lit-element';

export type UserUpdateEvent = CustomEvent<gapi.auth2.GoogleUser>;

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
  gAuth!: gapi.auth2.GoogleAuth;
  @property() private profile: gapi.auth2.BasicProfile | null = null;

  connectedCallback() {
    super.connectedCallback();
    this.onUserUpdate(this.gAuth.currentUser.get());
  }

  firstUpdated() {
    this.gAuth.currentUser.listen(this.onUserUpdate);
  }

  onclick = () => {
    if (this.gAuth.currentUser.get().isSignedIn()) {
      return this.gAuth.signOut();
    } else {
      return this.gAuth.signIn();
    }
  }

  private onUserUpdate = (user: gapi.auth2.GoogleUser) => {
    this.profile = user.isSignedIn() ? user.getBasicProfile() : null;
    this.dispatchEvent(new CustomEvent<gapi.auth2.GoogleUser>('user-update', {
      detail: user,
      composed: true,
    }));
  }

  protected render() {
    if (!this.profile) {
      return svg`
        <svg viewBox="0 0 24 24">
          <path
            d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0
            3c1.66 0 3 1.34 3 3s-1.34 3-3 3-3-1.34-3-3 1.34-3 3-3zm0 14.2c-2.5
            0-4.71-1.28-6-3.22.03-1.99 4-3.08 6-3.08 1.99 0 5.97 1.09 6 3.08-1.29
            1.94-3.5 3.22-6 3.22z"></path>
        </svg>
      `;
    }
    return html`
      <img title="Sign out of ${this.profile.getEmail()}" src="${this.profile.getImageUrl()}"/>
    `;
  }

  static styles = css`
    :host {
      display: inline-block;
      fill: red;
      cursor: pointer;
      height: 32px;
      width: 32px;
    }
    img {
      height: 100%;
      width: 100%;
      border-radius: 50%;
      overflow: hidden;
    }
  `;
}
