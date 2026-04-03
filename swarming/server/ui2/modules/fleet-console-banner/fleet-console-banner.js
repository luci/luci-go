// Copyright 2025 The LUCI Authors.
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
import { html, render } from "lit-html";

import "elements-sk/styles/buttons";

/**
 * @module swarming-ui/modules/fleet-console-banner
 * @description <h2><code>fleet-console-banner<code></h2>
 *
 * <p>
 *   Shows a banner prompting users to check out the new fleet console
 * </p>
 *
 */

const template = (ele) => html`
  <div class="fleet-console-banner">
    Fleet Console is now the recommended UI for tracking ChromeOS devices.
    <!-- the onclick is needed to synch the link with current query params -->
    <a role="link" href=${ele.link} @click=${ele.updateLink}>See these devices from FCon.</a>
  </div>
`;

window.customElements.define(
  "fleet-console-banner",
  class extends HTMLElement {
    constructor() {
      super();
      this.link = this._getLink();
    }

    updateLink() {
      this.link = this._getLink();
      this.render();
    }

    _getLink() {
      const urlPath = window.location.pathname;
      const queryParams = window.location.search;

      return (
        "https://ci.chromium.org/ui/fleet/redirects/swarming" +
        urlPath +
        queryParams
      );
    }

    connectedCallback() {
      this.render();
    }

    disconnectCallback() {}

    render() {
      render(template(this), this, { eventContext: this });
    }
  }
);
