// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import { errorMessage } from "elements-sk/errorMessage";
import { html, render } from "lit-html";

import { initPropertyFromAttrOrProperty } from "../util";

import "elements-sk/checkbox-sk";
import "elements-sk/styles/buttons";
import { TasksService } from "../services/tasks";
import { Timestamp } from "../task-list/task-list-helpers";

/**
 * @module swarming-ui/modules/task-mass-cancel
 * @description <h2><code>task-mass-cancel<code></h2>
 *
 * <p>
 * task-mass-cancel offers an interface for the user to cancel multiple tasks
 * (and hopefully avoid doing so on accident).
 * </p>
 *
 * @fires tasks-canceling-started
 * @fires tasks-canceling-finished
 */

const listItem = (tag) => html`<li>${tag}</li>`;

const template = (ele) => html`
  <div>
    You are about to cancel all PENDING bots with the following tags:
    <ul>
      ${ele.tags.map(listItem)}
    </ul>
    <div>
      <checkbox-sk
        ?checked=${ele._both}
        ?disabled=${ele._started}
        @click=${ele._toggleBoth}
        tabindex="0"
      >
      </checkbox-sk>
      Also include RUNNING tasks.
    </div>

    This is about ${ele._count()} tasks. Once you start the process, the only
    way to partially stop it is to close this browser window. If that sounds
    good, click the button below.
  </div>

  <button
    class="cancel"
    ?disabled=${!ele._readyToCancel || ele._started}
    @click=${ele._cancelAll}
  >
    Cancel the tasks
  </button>

  <div>
    <div ?hidden=${!ele._started}>
      Progress: ${ele._progress} canceled${ele._finished ? " - DONE." : "."}
    </div>
    <div>
      Note: tasks queued for cancellation will be canceled as soon as possible,
      but there may be some delay between when this dialog box is closed and all
      tasks actually being canceled.
    </div>
  </div>
`;

function fetchError(e, loadingWhat) {
  const message = `Unexpected error loading ${loadingWhat}: ${e.message}`;
  console.error(message);
  errorMessage(message, 5000);
}

const CANCEL_BATCH_SIZE = 100;

window.customElements.define(
  "task-mass-cancel",
  class extends HTMLElement {
    constructor() {
      super();
      this._readyToCancel = false;
      this._started = false;
      this._finished = false;
      this._both = false;
      this._progress = 0;
    }

    connectedCallback() {
      initPropertyFromAttrOrProperty(this, "authHeader");
      initPropertyFromAttrOrProperty(this, "end");
      initPropertyFromAttrOrProperty(this, "start");
      initPropertyFromAttrOrProperty(this, "tags");
      // Used for when default was loaded via attribute.
      if (typeof this.tags === "string") {
        this.tags = this.tags.split(",");
      }

      if (this.start) {
        this.start = Timestamp.fromMilliseconds(this.start);
      }
      if (this.end) {
        this.end = Timestamp.fromMilliseconds(this.end);
      }
      // sort for determinism
      this.tags.sort();

      this.render();
    }

    _cancelAll() {
      this._started = true;
      this.dispatchEvent(
        new CustomEvent("tasks-canceling-started", { bubbles: true })
      );
      this.render();

      const payload = {
        limit: CANCEL_BATCH_SIZE,
        tags: this.tags,
        start: this.start,
        end: this.end,
      };

      if (this._both) {
        payload.killRunning = true;
      }

      const service = new TasksService(this.authHeader);

      const maybeCancelMore = (resp) => {
        this._progress += parseInt(resp.matched || 0);
        this.render();
        if (resp.cursor) {
          const payload = {
            limit: CANCEL_BATCH_SIZE,
            tags: this.tags,
            start: this.start,
            end: this.end,
            cursor: resp.cursor,
          };
          if (this._both) {
            payload.killRunning = true;
          }
          service
            .massCancel(payload)
            .then(maybeCancelMore)
            .catch((e) => fetchError(e, "task-mass-cancel/cancel (paging)"));
        } else {
          this._finished = true;
          this.render();
          this.dispatchEvent(
            new CustomEvent("tasks-canceling-finished", { bubbles: true })
          );
        }
      };

      service
        .massCancel(payload)
        .then(maybeCancelMore)
        .catch((e) => fetchError(e, "task-mass-cancel/cancel"));
    }

    _count() {
      if (
        this._pendingCount === undefined ||
        this._runningCount === undefined
      ) {
        return "...";
      }
      if (this._both) {
        return this._pendingCount + this._runningCount;
      }
      return this._pendingCount;
    }

    _fetchCount() {
      if (!this.authHeader) {
        // This should never happen
        console.warn("no authHeader received, try refreshing the page?");
        return;
      }
      const service = new TasksService(this.authHeader);

      const pendingParams = {
        state: "QUERY_PENDING",
        tags: this.tags,
        // Search in the last week to get the count.  PENDING tasks should expire
        // well before then, so this should be pretty accurate.
        start: Timestamp.hoursAgo(24 * 7),
        end: new Date(),
      };

      const pendingPromise = service
        .count(pendingParams)
        .then((resp) => {
          this._pendingCount = parseInt(resp.count || 0);
        })
        .catch((e) => fetchError(e, "task-mass-cancel/pending"));

      const runningParams = {
        state: "QUERY_RUNNING",
        tags: this.tags,
        // Search in the last week to get the count.  RUNNING tasks should finish
        // well before then, so this should be pretty accurate.
        start: Timestamp.hoursAgo(7 * 24),
        end: new Date(),
      };

      const runningPromise = service
        .count(runningParams)
        .then((resp) => {
          this._runningCount = parseInt(resp.count || 0);
        })
        .catch((e) => fetchError(e, "task-mass-cancel/running"));

      // re-render when both have returned
      Promise.all([pendingPromise, runningPromise]).then(() => {
        this._readyToCancel = true;
        this.render();
      });
    }

    render() {
      render(template(this), this, { eventContext: this });
    }

    /** show prepares the UI to be shown to the user */
    show() {
      this._readyToCancel = false;
      this._started = false;
      this._finished = false;
      this._progress = 0;
      this._fetchCount();
      this.render();
    }

    _toggleBoth(e) {
      // This prevents a double event from happening.
      e.preventDefault();
      if (this._started) {
        return;
      }
      this._both = !this._both;
      this.render();
    }
  }
);
