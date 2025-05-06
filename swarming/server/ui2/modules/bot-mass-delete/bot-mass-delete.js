// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import { errorMessage } from "elements-sk/errorMessage";
import { html, render } from "lit-html";
import { until } from "lit-html/directives/until";

import { initPropertyFromAttrOrProperty } from "../util";

import "elements-sk/styles/buttons";
import { BotsService } from "../services/bots";

/**
 * @module swarming-ui/modules/bot-mass-delete
 * @description <h2><code>bot-mass-delete<code></h2>
 *
 * <p>
 *   bot-mass-delete offers an interface for the user to delete multiple bots
 *   (and hopefully avoid doing so on accident). Care is taken such that only dead
 *   bots are deleted.
 * </p>
 *
 * @fires bots-deleting-started
 * @fires bots-deleting-finished
 */

const listItem = (dim) => html`<li>${dim.key}:${dim.value}</li>`;

const template = (ele) => html`
  <div>
    You are about to delete all DEAD bots with the following dimensions:
    <ul>
      ${ele.dimensions.map(listItem)}
    </ul>

    This is about ${ele._count} bots. Once you start the process, the only way
    to partially stop it is to close this browser window. If that sounds good,
    click the button below.
  </div>

  <button
    class="delete"
    ?disabled=${!ele._readyToDelete || ele._started}
    @click=${ele._deleteAll}
    tabindex="0"
  >
    Delete the bots
  </button>

  <div>
    <div ?hidden=${!ele._started}>
      Progress: ${ele._progress} deleted${ele._finished ? " - DONE." : "."}
    </div>
    <div>
      Note: the bot deletion is being done in browser - closing the window will
      stop the mass deletion.
    </div>
  </div>
`;

function fetchError(e, loadingWhat) {
  const message = `Unexpected error loading ${loadingWhat}: ${e.message}`;
  console.error(message);
  errorMessage(message, 5000);
}

window.customElements.define(
  "bot-mass-delete",
  class extends HTMLElement {
    constructor() {
      super();
      this._count = "...";
      this._readyToDelete = false;
      this._started = false;
      this._finished = false;
      this._progress = 0;
      // This is set to undefined so that it picks up the initial
      // value sent through attributes on the first load.
      // See initPropertyFromAttrOrProperty
      this._dimensions = undefined;
    }

    connectedCallback() {
      initPropertyFromAttrOrProperty(this, "authHeader");
      initPropertyFromAttrOrProperty(this, "dimensions");
      this.render();
    }

    set dimensions(newVal) {
      if (typeof newVal === "string") {
        newVal = newVal.split(",");
      }
      // sort for determinism
      newVal.sort();
      this._dimensions = newVal.map((dim) => {
        const [key, value] = dim.split(":");
        return { key, value };
      });
    }

    get dimensions() {
      return this._dimensions;
    }

    _deleteAll() {
      this._started = true;
      this.dispatchEvent(
        new CustomEvent("bots-deleting-started", { bubbles: true })
      );

      const request = {
        dimensions: this.dimensions,
        limit: 200, // see https://crbug.com/908423
        isDead: "TRUE",
      };

      const service = new BotsService(this.authHeader);
      let bots = [];

      service
        .list(request)
        .then((resp) => {
          const maybeLoadMore = (resp) => {
            bots = bots.concat(resp.items || []);
            this.render();
            if (resp.cursor) {
              const request = {
                cursor: resp.cursor,
                dimensions: this.dimensions,
                limit: 200, // see https://crbug.com/908423
                isDead: "TRUE",
              };
              service
                .list(request)
                .then(maybeLoadMore)
                .catch((e) => fetchError(e, "bot-mass-delete/list (paging)"));
            } else {
              const deleteNext = (bots) => {
                if (!bots.length) {
                  this._finished = true;
                  this.render();
                  this.dispatchEvent(
                    new CustomEvent("bots-deleting-finished", {
                      bubbles: true,
                    })
                  );
                  return;
                }
                const toDelete = bots.pop();
                service
                  .delete(toDelete.botId)
                  .then(() => {
                    this._progress++;
                    this.render();
                    deleteNext(bots);
                  })
                  .catch((e) => fetchError(e, "bot-mass-delete/delete"));
              };
              deleteNext(bots);
            }
          };
          maybeLoadMore(resp);
        })
        .catch((e) => fetchError(e, "bot-mass-delete/list"));

      this.render();
    }

    _fetchCount() {
      if (!this.authHeader) {
        // This should never happen
        console.warn("no authHeader received, try refreshing the page?");
        return;
      }
      const service = new BotsService(this.authHeader);
      const countPromise = service
        .count(this.dimensions)
        .then((resp) => {
          this._readyToDelete = true;
          this.render();
          return parseInt(resp.dead || 0);
        })
        .catch((e) => fetchError(e, "bot-mass-delete/count"));
      this._count = html`${until(countPromise, "...")}`;
    }

    render() {
      render(template(this), this, { eventContext: this });
    }

    /** show prepares the UI to be shown to the user */
    show() {
      this._readyToDelete = false;
      this._started = false;
      this._finished = false;
      this._progress = 0;
      this._fetchCount();
      this.render();
    }
  }
);
