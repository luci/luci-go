// Copyright 2018 The LUCI Authors.
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

/** @module swarming-ui/modules/oauth-login
 * @description <h2><code>oauth-login</code></h2>
 * oauth-login is a small widget that handles the login flow.
 *
 * <p>
 *  This widget either a sign in button or displays the info of the
 *  logged-in user and a sign out button. The widget fires an event
 *  when the user has logged in and clients should be able to make
 *  authenticated requests.
 * </p>
 *
 * @prop testing_offline - If true, the real login flow won't be used.
 *    Instead, dummy data will be used. Ideal for local testing.
 *
 * @evt log-in The event that is fired when the user has logged in
 *             has a detail of the form:
 *
 * <pre>
 * {
 *   authHeader: "Bearer abc12d",
 *   profile: {
 *     email: 'foo@example.com',
 *     imageURL: 'http://example.com/img.png',
 *   },
 *   refresh: false,
 * }
 * </pre>
 * where authHeader is a string that should be used as the
 * "Authorization" header for authenticated requests, and refresh is true
 * when an existing session refreshed its token for the same user.
 *
 */

import { html, render } from "lit-html";
import { upgradeProperty } from "elements-sk/upgradeProperty";
import { errorMessage } from "elements-sk/errorMessage";

import { jsonOrThrow } from "common-sk/modules/jsonOrThrow";

const template = (ele) => {
  if (ele.authHeader) {
    return html` <div>
      <img
        class="center"
        id="avatar"
        src="${ele._profile.imageURL}"
        width="30"
        height="30"
      />
      <span class="center">${ele._profile.email}</span>
      <span class="center">|</span>
      <a class="center" @click=${ele._logOut} href="#">Sign out</a>
    </div>`;
  } else {
    return html` <div>
      <a @click=${ele._logIn} href="#">Sign in</a>
    </div>`;
  }
};

window.customElements.define(
  "oauth-login",
  class extends HTMLElement {
    connectedCallback() {
      upgradeProperty(this, "testing_offline");
      this._auth_header = "";
      this._profile = null;
      this._refreshTimer = null;
      if (!this.testing_offline) {
        this._refreshAuthState();
      }
      this.render();
    }

    disconnectedCallback() {
      this._fetchGen = (this._fetchGen || 0) + 1;
      clearTimeout(this._refreshTimer);
      this._refreshTimer = null;
    }

    static get observedAttributes() {
      return ["testing_offline"];
    }

    /** @prop {string} authHeader the "Authorization" header that should be used
     *                  for authenticated requests. Read-only. */
    get authHeader() {
      return this._auth_header;
    }

    /** @prop {Object} profile An object with keys email and imageURL of the
                             logged in user. Read Only. */
    get profile() {
      return this._profile;
    }

    /** @prop {bool} testing_offline Mirrors the attribute 'testing_offline'. */
    // eslint-disable-next-line camelcase
    get testing_offline() {
      return this.hasAttribute("testing_offline");
    }
    // eslint-disable-next-line camelcase
    set testing_offline(val) {
      if (val) {
        this.setAttribute("testing_offline", true);
      } else {
        this.removeAttribute("testing_offline");
      }
    }

    _fireLoginEvent(authState) {
      const prevEmail = this._profile && this._profile.email;
      const nextEmail = authState && authState.email;
      const refresh =
        !!this._auth_header &&
        !!(authState && authState.accessToken) &&
        !!prevEmail &&
        prevEmail === nextEmail;
      if (authState && authState.accessToken) {
        this._profile = {
          email: authState.email,
          imageURL: authState.picture,
        };
        this._auth_header = `Bearer ${authState.accessToken}`;
      } else {
        this._profile = null;
        this._auth_header = "";
      }
      this.dispatchEvent(
        new CustomEvent("log-in", {
          detail: {
            authHeader: this._auth_header,
            profile: this._profile,
            refresh: refresh,
          },
          bubbles: true,
        })
      );
    }

    _logIn() {
      if (this.testing_offline) {
        this._fireLoginEvent({
          email: "missing@chromium.org",
          picture:
            "http://storage.googleapis.com/gd-wagtail-prod-assets/original_images/logo_google_fonts_color_2x_web_64dp.png",
          accessToken: "12345678910-boomshakalaka",
        });
        this.render();
      } else {
        // This will eventually reload the page with the session cookie set.
        this._nagivateTo("login");
      }
    }

    _logOut() {
      if (this.testing_offline) {
        // Just reload the page, it is logged out by default in offline mode.
        window.location.reload();
      } else {
        // Navigate to the endpoint that removes the session cookie.
        this._nagivateTo("logout");
      }
    }

    _nagivateTo(action) {
      const back = window.location.pathname + window.location.search;
      if (back && back != "/") {
        window.location = `/auth/openid/${action}?r=${encodeURIComponent(
          back
        )}`;
      } else {
        window.location = `/auth/openid/${action}`;
      }
    }

    _fetchAuthState() {
      // This backend will fetch the OAuth tokens using the session identified
      // by the session cookie set during the login flow.
      const options = {
        mode: "same-origin",
        credentials: "same-origin",
        cache: "no-store",
      };
      return fetch("/auth/openid/state", options).then(jsonOrThrow);
    }

    _refreshAuthState() {
      if (this.testing_offline) {
        return;
      }
      clearTimeout(this._refreshTimer);
      this._refreshTimer = null;
      const fetchGen = (this._fetchGen = (this._fetchGen || 0) + 1);
      this._fetchAuthState().then(
        (authState) => {
          if (!this.isConnected || fetchGen !== this._fetchGen) {
            return;
          }
          if (
            authState &&
            authState.identity !== "anonymous:anonymous" &&
            authState.accessToken
          ) {
            this._scheduleRefresh(authState);
            this._fireLoginEvent(authState);
            this.render();
          } else if (this._auth_header) {
            this._fireLoginEvent(null);
            this.render();
          }
        },
        (error) => {
          if (!this.isConnected || fetchGen !== this._fetchGen) {
            return;
          }
          console.error(error);
          if (this._auth_header) {
            this._refreshTimer = setTimeout(() => {
              this._refreshTimer = null;
              this._refreshAuthState();
            }, 15 * 1000);
          } else {
            errorMessage(
              `Error getting auth state: ${JSON.stringify(error)}`,
              10000
            );
          }
        }
      );
    }

    _scheduleRefresh(authState) {
      let expiresIn = authState.accessTokenExpiresIn;
      if (
        !Number.isFinite(expiresIn) &&
        Number.isFinite(authState.accessTokenExpiry) &&
        authState.accessTokenExpiry > 0
      ) {
        expiresIn = authState.accessTokenExpiry - Date.now() / 1000;
      }
      if (!Number.isFinite(expiresIn)) {
        return;
      }
      // Refresh 2 minutes before expiry, clamped between 10s and 1h.
      const delaySec = Math.min(Math.max(10, expiresIn - 120), 3600);
      this._refreshTimer = setTimeout(() => {
        this._refreshTimer = null;
        this._refreshAuthState();
      }, delaySec * 1000);
    }

    render() {
      render(template(this), this, { eventContext: this });
    }

    attributeChangedCallback(attrName, oldVal, newVal) {
      this.render();
    }
  }
);
