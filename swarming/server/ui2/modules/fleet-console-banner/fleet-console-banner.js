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

export const BANNER_CONFIG = {
  "chromeos-swarming.appspot.com": {
    text: "Fleet Console is now the recommended UI for tracking ChromeOS devices.",
    linkText: "See these devices from FCon.",
  },
  "chromium-swarm.appspot.com": {
    text: "Browser devices can now be viewed from the Fleet Console!",
    linkText: "View these devices in FCon here",
    platform: "chromium",
  },
  "chrome-swarming.appspot.com": {
    text: "Browser devices can now be viewed from the Fleet Console!",
    linkText: "View these devices in FCon here",
    platform: "chromium",
  },
  "chromium-swarm-dev.appspot.com": {
    text: "Browser devices can now be viewed from the Fleet Console!",
    linkText: "View these devices in FCon here",
    platform: "chromium",
  },
};

export function getMockableHostname() {
  const hostname = window.location.hostname;
  const isLocal = hostname === "localhost" || hostname === "127.0.0.1";
  if (!isLocal) {
    return hostname;
  }
  const urlParams = new URLSearchParams(window.location.search);
  return urlParams.get("mock_hostname") || hostname;
}

export function getProjectId(hostname) {
  const match = hostname.match(/^([^.]+)\.appspot\.com$/);
  return match ? match[1] : "not_found";
}

const template = (ele) => html`
  <div class="fleet-console-banner">
    ${ele.config.text || ""}
    <!-- the onclick is needed to synch the link with current query params -->
    <a role="link" href=${ele.link} @click=${ele.updateLink}>
      ${ele.config.linkText || ""}
    </a>
  </div>
`;

window.customElements.define(
  "fleet-console-banner",
  class extends HTMLElement {
    constructor() {
      super();
      const hostname = getMockableHostname();
      this.config = BANNER_CONFIG[hostname] || {};
      this.link = this._getLink(hostname);
    }

    updateLink() {
      const hostname = getMockableHostname();
      this.link = this._getLink(hostname);
      this.render();
    }

    _getLink(hostname) {
      const urlPath = window.location.pathname;
      let queryParams = window.location.search;

      const config = BANNER_CONFIG[hostname];
      if (config && config.platform) {
        const params = new URLSearchParams(queryParams);
        params.set("platform", config.platform);
        queryParams = "?" + params.toString();
      }

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
