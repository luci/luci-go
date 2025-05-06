// Copyright 2025 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.
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
    We're dogfooding a new UI for managing ChromeOS devices. Click
    <!-- the onclick is needed to synch the link with current query params -->
    <a role="link" href=${ele.link} @click=${ele.updateLink}>here</a>
    to see these devices from the Fleet Console.
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
