// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import { $$ } from "common-sk/modules/dom";
import { errorMessage } from "elements-sk/errorMessage";
import { html } from "lit-html";
import { ifDefined } from "lit-html/directives/if-defined";
import { stateReflector } from "common-sk/modules/stateReflector";

import "elements-sk/checkbox-sk";
import "elements-sk/icon/add-circle-outline-icon-sk";
import "elements-sk/icon/remove-circle-outline-icon-sk";
import "elements-sk/styles/buttons";
import "../bot-page-summary";
import "../dialog-pop-over";
import "../fleet-console-banner";
import "../swarming-app";

import {
  parseBotData,
  parseEvents,
  parseTasks,
  quarantineMessage,
  siblingBotsLink,
} from "./bot-page-helpers";
import { stateClass as taskClass } from "../task-page/task-page-helpers";
import { timeDiffApprox, timeDiffExact, taskPageLink, humanize } from "../util";
import SwarmingAppBoilerplate from "../SwarmingAppBoilerplate";

/**
 * @module swarming-ui/modules/bot-page
 * @description <h2><code>bot-page<code></h2>
 *
 * <p>
 *   Bot Page shows the information about a bot, including events and tasks.
 * </p>
 *
 * <p>This is a top-level element.</p>
 *
 * @attr testing_offline - If true, the real login flow won't be used.
 *    Instead, dummy data will be used. Ideal for local testing.
 */

const idAndButtons = (ele) => {
  if (!ele._botId) {
    return html`
<div class=id_buttons>
  <input id=id_input placeholder="Bot ID" @change=${ele._updateID}></input>
  <span class=message>Enter a Bot ID to get started.</span>
</div>`;
  }
  return html`
<div class=id_buttons>
  <input id=id_input placeholder="Bot ID" @change=${ele._updateID}></input>
  <button title="Refresh data" class=refresh
          @click=${ele._refresh}>refresh</button>
</div>`;
};

const statusAndTask = (ele, bot) => {
  if (!ele._botId) {
    return "";
  }
  // Using the hidden classes instead of the attribute lets us
  // more easily default to hidden (.hidden) when the data
  // is loading, so the elements are not shown prematurely.
  return html` <tr
      class="dead ${bot.deleted ? "" : "hidden"}"
      title="This bot was deleted."
    >
      <td colspan="3">THIS BOT WAS DELETED</td>
    </tr>
    <tr class=${bot.isDead ? "dead" : ""}>
      <td>Last Seen</td>
      <td title=${bot.humanized.lastSeenTs}>
        ${timeDiffExact(bot.lastSeenTs)} ago
      </td>
      <td>
        <button
          class="shut_down ${!bot.isDead && bot.firstSeenTs ? "" : "hidden"}"
          ?hidden=${bot.isDead}
          ?disabled=${!ele.permissions.terminateBot}
          @click=${ele._promptShutdown}
        >
          Stop the bot gracefully
        </button>
        <button
          class="delete ${bot.isDead && !bot.deleted ? "" : "hidden"}"
          ?disabled=${!ele.permissions.deleteBot}
          @click=${ele._promptDelete}
        >
          Delete
        </button>
      </td>
    </tr>
    <tr class="quarantined ${bot.quarantined ? "" : "hidden"}">
      <td>Quarantined</td>
      <td colspan="2" class="code">${quarantineMessage(bot)}</td>
    </tr>
    <tr class="dead ${bot.isDead && !bot.deleted ? "" : "hidden"}">
      <td>Dead</td>
      <td colspan="2" class="code">
        Bot has been missing longer than 10 minutes
      </td>
    </tr>
    <tr class="maintenance ${bot.maintenanceMsg ? "" : "hidden"}">
      <td>In Maintenance</td>
      <td colspan="2" class="code">${bot.maintenanceMsg}</td>
    </tr>
    <tr>
      <td>${bot.isDead ? "Died on Task" : "Current Task"}</td>
      <td>
        <a
          target="_blank"
          rel="noopener"
          href=${ifDefined(taskPageLink(bot.taskId))}
        >
          ${bot.taskId || "idle"}
        </a>
      </td>
      <td>
        <button
          class="kill"
          ?hidden=${!bot.taskId || bot.isDead}
          ?disabled=${!ele.permissions.cancelTask}
          @click=${ele._promptKill}
        >
          Kill task
        </button>
      </td>
    </tr>`;
};

const dimensionBlock = (dimensions) => html`
  <tr>
    <td rowspan=${dimensions.length + 1}>
      <a href=${siblingBotsLink(dimensions)}> Dimensions </a>
    </td>
  </tr>
  ${dimensions.map(dimensionRow)}
`;

const dimensionRow = (dimension) => html`
  <tr>
    <td>${dimension.key}</td>
    <td>${dimension.value.join(" | ")}</td>
  </tr>
`;

const dataAndMPBlock = (ele, bot) => html`
  <tr title="IP address that the server saw the connection from.">
    <td>External IP</td>
    <td colspan="2">
      <a href=${"http://" + bot.externalIp}>${bot.externalIp}</a>
    </td>
  </tr>
  <tr
    class=${ele.serverDetails.botVersion === bot.version ? "" : "old_version"}
    title="Version is based on the content of swarming_bot.zip which is the swarming bot code.
           The bot won't update if quarantined, dead, or busy."
  >
    <td>Bot Version</td>
    <td colspan="2">${bot.version && bot.version.substring(0, 10)}</td>
  </tr>
  <tr title="The version the server expects the bot to be using.">
    <td>Expected Bot Version</td>
    <td colspan="2">
      ${ele.serverDetails.botVersion &&
      ele.serverDetails.botVersion.substring(0, 10)}
    </td>
  </tr>
  <tr title="First time ever a bot with this id contacted the server.">
    <td>First seen</td>
    <td colspan="2" title=${bot.humanized.firstSeenTs}>
      ${timeDiffApprox(bot.firstSeenTs)} ago
    </td>
  </tr>
  <tr title="How the bot is authenticated by the server.">
    <td>Authenticated as</td>
    <td colspan="2">${bot.authenticatedAs}</td>
  </tr>
`;

const deviceSection = (ele, bot) => {
  if (!bot.device_list || !bot.device_list.length) {
    return "";
  }
  // At the moment, this only supports Android devices
  // It would be nice to handle other devices, like Chromebooks.
  // https://crbug.com/814515
  return html` <h2>Android Devices</h2>

    <table class="devices">
      <thead>
        <tr>
          <th>ID</th>
          <th>Battery</th>
          <th>Avg Temp. (°C)</th>
          <th>State</th>
        </tr>
      </thead>
      <tbody>
        ${bot.device_list.map(deviceRow)}
      </tbody>
    </table>`;
};

const deviceRow = (device) => html`
  <tr>
    <td>${device.id}</td>
    <td>${(device.battery && device.battery.level) || "???"}</td>
    <td>${device.averageTemp}</td>
    <td>${device.state}</td>
  </tr>
`;

const stateSection = (ele, bot) => html`
  <span class="title">State</span>
  <button class="state" @click=${ele._toggleBotState}>
    <add-circle-outline-icon-sk
      ?hidden=${ele._showState}
    ></add-circle-outline-icon-sk>
    <remove-circle-outline-icon-sk
      ?hidden=${!ele._showState}
    ></remove-circle-outline-icon-sk>
  </button>

  <div ?hidden=${!ele._showState} class="bot_state">
    ${JSON.stringify((bot && bot.state) || {}, null, 2)}
  </div>
`;

const tasksTable = (ele, tasks) => {
  if (
    !ele.loggedInAndAuthorized ||
    !ele._botId ||
    ele._showEvents ||
    ele._notFound
  ) {
    return "";
  }
  return html`
    <table class="tasks_table">
      <thead>
        <tr>
          <th>Task</th>
          <th>Started</th>
          <th>Duration</th>
          <th>Result</th>
        </tr>
      </thead>
      <tbody>
        ${tasks.map(taskRow)}
      </tbody>
    </table>

    <button
      class="more_tasks"
      ?disabled=${!ele._taskCursor}
      @click=${ele._moreTasks}
    >
      Show More
    </button>
  `;
};

const taskRow = (task) => html`
  <tr class=${taskClass(task)}>
    <td class="break-all">
      <a target="_blank" rel="noopener" href=${taskPageLink(task.taskId)}>
        ${task.name}
      </a>
    </td>
    <td>${task.humanized.time.startedTs}</td>
    <td title=${task.humanized.time.completedTs}>${task.humanTotalDuration}</td>
    <td>${task.humanState}</td>
  </tr>
`;

const eventsTable = (ele, events) => {
  if (
    !ele.loggedInAndAuthorized ||
    !ele._botId ||
    !ele._showEvents ||
    ele._notFound
  ) {
    return "";
  }
  return html`
    <div class="all-events">
      <checkbox-sk ?checked=${ele._showAll} @click=${ele._toggleShowAll}>
      </checkbox-sk>
      <span>Show all events</span>
    </div>
    <table class="events_table">
      <thead>
        <tr>
          <th>Message</th>
          <th>Type</th>
          <th>Timestamp</th>
          <th>Task ID</th>
          <th>Version</th>
        </tr>
      </thead>
      <tbody>
        ${events.map((event) =>
          eventRow(event, ele._showAll, ele.serverDetails.botVersion)
        )}
      </tbody>
    </table>

    <button
      class="more_events"
      ?disabled=${!ele._eventsCursor}
      @click=${ele._moreEvents}
    >
      Show More
    </button>
  `;
};

const eventRow = (event, showAll, serverVersion) => {
  if (!showAll && !event.message) {
    return "";
  }
  const msg = [event.message, event.maintenanceMsg, event.state.quarantined]
    .filter(Boolean)
    .join("\n");
  return html` <tr>
    <td class="message">${msg}</td>
    <td>${event.eventType}</td>
    <td>${event.humanized.time.ts}</td>
    <td>
      <a target="_blank" rel="noopener" href=${taskPageLink(event.taskId)}>
        ${event.taskId}
      </a>
    </td>
    <td class=${serverVersion === event.version ? "" : "old_version"}>
      ${event.version && event.version.substring(0, 10)}
    </td>
  </tr>`;
};

const otherPrompts = (ele) =>
  html` <dialog-pop-over>
    <div class="prompt-dialog content">
      Are you sure you want to ${ele._prompt}?
      <div class="horizontal layout end">
        <button @click=${ele._closePopup} class="cancel" tabindex="0">
          NO
        </button>
        <button @click=${ele._promptCallback} class="ok" tabindex="0">
          YES
        </button>
      </div>
    </div>
  </dialog-pop-over>`;

const terminatePrompt = (ele) =>
  html` <dialog-pop-over>
    <div class="prompt-dialog content">
      <div class="vertical grid reason">
        <div>Are you sure you want to ${ele._prompt}?</div>
        <div class="horizontal layout terminate">
          <label for="reason">Shutdown reason (optional)</label>
          <input type="text" id="reason" name="reason" />
        </div>
        <div class="horizontal layout end">
          <button @click=${ele._closePopup} class="cancel" tabindex="0">
            NO
          </button>
          <button @click=${ele._promptCallback} class="ok" tabindex="0">
            YES
          </button>
        </div>
      </div>
    </div>
  </dialog-pop-over>`;

const prompt = (ele) => {
  if (ele._promptType === "shutdown") return terminatePrompt(ele);
  else return otherPrompts(ele);
};

const template = (ele) => html`
<swarming-app id=swapp
              ?testing_offline=${ele.testing_offline}>
  <header>
    <div class=title>Swarming Bot Page</div>
      <aside class=hideable>
        <a href=/>Home</a>
        <a href=/botlist>Bot List</a>
        <a href=/tasklist>Task List</a>
        <a href=/task>Task Page</a>
      </aside>
  </header>
  <main>
    <h2 class=message ?hidden=${ele.loggedInAndAuthorized}>${ele._message}</h2>
    <fleet-console-banner
      ?hidden=${ele._projectId !== "chromeos-swarming" ||
      // Fleet Console uses the `dut_name` to retrieve ChromeOS device details,
      // so hide the banner if that information isn't available.
      !(
        ele._bot &&
        ele._bot.dimensions &&
        ele._bot.dimensions.some((d) => d.key === "dut_name")
      )}
    ></fleet-console-banner>

    <div class=top ?hidden=${!ele.loggedInAndAuthorized}>
      ${idAndButtons(ele)}
      <h2 class=not_found ?hidden=${!ele._notFound || !ele._botId}>
        Bot not found
      </h2>
    </div>
    <div class="horizontal layout wrap content"
         ?hidden=${!ele.loggedInAndAuthorized || !ele._botId || ele._notFound}>
      <div class=grow>
        <table class=data_table>
          ${statusAndTask(ele, ele._bot)}
          ${dimensionBlock(ele._bot.dimensions || [])}
          ${dataAndMPBlock(ele, ele._bot)}
        </table>
        ${deviceSection(ele, ele._bot)}
        ${stateSection(ele, ele._bot)}
      </div>

      <div class="stats grow">
        <bot-page-summary .tasks=${ele._tasks}></bot-page-summary>
      </div>
    </div>

    <div class=tasks-events-picker
         ?hidden=${!ele.loggedInAndAuthorized || !ele._botId || ele._notFound}>
      <div class=tab
           @click=${(e) => ele._setShowEvents(false)}
           ?selected=${!ele._showEvents}>
        Tasks
      </div>
      <div class=tab
           @click=${(e) => ele._setShowEvents(true)}
           ?selected=${ele._showEvents}>
        Events
      </div>
    </div>

    ${tasksTable(ele, ele._tasks)}
    ${eventsTable(ele, ele._events)}

  </main>
  <footer></footer>
  ${prompt(ele)}
</swarming-app>
`;

window.customElements.define(
  "bot-page",
  class extends SwarmingAppBoilerplate {
    constructor() {
      super(template);

      // Set empty values to allow empty rendering while we wait for
      // stateReflector (which triggers on DomReady). Additionally, these values
      // help stateReflector with types.
      this._botId = "";
      this._showState = false;
      this._showEvents = false;
      this._showAll = false;

      this._urlParamsLoaded = false;
      this._stateChanged = stateReflector(
        /* getState*/ () => {
          return {
            // provide empty values
            id: this._botId,
            s: this._showState,
            e: this._showEvents,
            a: this._showAll,
          };
        },
        /* setState*/ (newState) => {
          // default values if not specified.
          this._botId = newState.id || this._botId;
          this._showState = newState.s; // default to false
          this._showEvents = newState.e; // default to false
          this._showAll = newState.a; // default to false
          this._urlParamsLoaded = true;
          this._fetch();
          this.render();
        }
      );

      this._bot = humanize({});
      this._notFound = false;
      this._tasks = [];
      this._events = [];
      this._resetCursors();

      this._promptCallback = () => {};

      this._message = "You must sign in to see anything useful.";
      // Allows us to abort fetches that are tied to the id when the id changes.
      this._fetchController = null;

      const idx = location.hostname.indexOf(".appspot.com");
      this._projectId = location.hostname.substring(0, idx) || "not_found";
    }

    connectedCallback() {
      super.connectedCallback();

      this._loginEvent = (e) => {
        this._fetch();
        this.render();
      };
      this.addEventListener("log-in", this._loginEvent);
      this.render();
    }

    disconnectedCallback() {
      super.disconnectedCallback();
      this.removeEventListener("log-in", this._loginEvent);
    }

    _closePopup() {
      $$("dialog-pop-over", this).hide();
    }

    _deleteBot() {
      this.app.addBusyTasks(1);
      this._createBotService()
        .delete(this._botId)
        .then((_response) => {
          this._closePopup();
          errorMessage("Request to delete bot sent", 4000);
          this.render();
          this.app.finishedTask();
        })
        .catch((e) => {
          this._closePopup();
          this.prpcError(e, "bot/delete"); // calls app.finishedTask()
          this.render();
        });
    }

    _fetch() {
      if (
        !this.loggedInAndAuthorized ||
        !this._urlParamsLoaded ||
        !this._botId
      ) {
        return;
      }
      if (this._fetchController) {
        // Kill any outstanding requests.
        this._fetchController.abort();
      }
      // Make a fresh abort controller for each set of fetches. AFAIK, they
      // cannot be reused once aborted.
      this._fetchController = new AbortController();
      const extra = {
        authHeader: this.authHeader,
        signal: this._fetchController.signal,
      };
      // re-fetch permissions with the bot ID.
      this.app._fetchPermissions(extra, { botId: this._botId });
      this.app.addBusyTasks(1);
      const botService = this._createBotService();
      botService
        .bot(this._botId)
        .then((resp) => {
          this._notFound = false;
          this._bot = parseBotData(resp);
          this.render();
          this.app.finishedTask();
        })
        .catch((e) => {
          if (e.codeName === "NOT_FOUND") {
            this._bot = humanize({});
            this._notFound = true;
            this.render();
          }
          this.prpcError(e, "bot/data");
        });
      if (!this._taskCursor) {
        this.app.addBusyTasks(1);
        botService
          .tasks(this._botId, this._taskCursor)
          .then((resp) => {
            this._taskCursor = resp.cursor;
            this._tasks = parseTasks(resp.items);
            this.render();
            this.app.finishedTask();
          })
          .catch((e) => this.prpcError(e, "bot/tasks"));
      }

      if (!this._eventsCursor) {
        this.app.addBusyTasks(1);
        botService
          .events(this._botId, this._eventsCursor)
          .then((resp) => {
            this._eventsCursor = resp.cursor;
            this._events = parseEvents(resp.items);
            this.render();
            this.app.finishedTask();
          })
          .catch((e) => this.prpcError(e, "bot/events"));
      }
    }

    _killTask() {
      this.app.addBusyTasks(1);
      this._createTasksService()
        .cancel(this._bot.taskId, true)
        .then((_response) => {
          this._closePopup();
          errorMessage("Request to kill task sent", 4000);
          this.render();
          this.app.finishedTask();
        })
        .catch((e) => {
          this._closePopup();
          this.prpcError(e, "task/kill"); // calls app.finishedTask()
          this.render();
        });
    }

    _moreEvents() {
      if (!this._eventsCursor) {
        return;
      }
      this.app.addBusyTasks(1);
      this._createBotService()
        .events(this._botId, this._eventsCursor)
        .then((resp) => {
          this._eventsCursor = resp.cursor;
          this._events.push(...parseEvents(resp.items));
          this.render();
          this.app.finishedTask();
        })
        .catch((e) => this.fetchError(e, "bot/more_events"));
    }

    _moreTasks() {
      if (!this._taskCursor) {
        return;
      }
      this.app.addBusyTasks(1);
      this._createBotService()
        .tasks(this._botId, this._taskCursor)
        .then((resp) => {
          this._taskCursor = resp.cursor;
          this._tasks.push(...parseTasks(resp.items));
          this.render();
          this.app.finishedTask();
        })
        .catch((e) => this.prpcError(e, "bot/more_tasks"));
    }

    _promptDelete() {
      this._promptType = "delete";
      this._prompt = `delete dead bot '${this._botId}'`;
      this._promptCallback = this._deleteBot;
      this.render();

      $$("dialog-pop-over", this).show();
      $$("dialog-pop-over button.cancel", this).focus();
    }

    _promptKill() {
      this._promptType = "kill";
      this._prompt = `kill running task '${this._bot.taskId}'`;
      this._promptCallback = this._killTask;
      this.render();

      $$("dialog-pop-over", this).show();
      $$("dialog-pop-over button.cancel", this).focus();
    }

    _promptShutdown() {
      this._promptType = "shutdown";
      this._prompt = `gracefully shut down bot '${this._botId}'`;
      this._promptCallback = this._shutdownBot;
      this.render();

      $$("dialog-pop-over", this).show();
      $$("dialog-pop-over button.cancel", this).focus();
    }

    _refresh() {
      this._resetCursors();
      this._fetch();
      this.render();
    }

    render() {
      super.render();
      const idInput = $$("#id_input", this);
      idInput.value = this._botId;
    }

    // _resetCursors indicates we should forget any tasks and events we have
    // seen and start over (when _fetch() is next called).
    _resetCursors() {
      this._taskCursor = "";
      this._eventsCursor = "";
    }

    _setShowEvents(shouldShow) {
      this._showEvents = shouldShow;
      this._stateChanged();
      this.render();
    }

    _shutdownBot() {
      this.app.addBusyTasks(1);
      const reason = $$("#reason").value;
      this._createBotService()
        .terminate(this._botId, reason)
        .then((_resp) => {
          this._closePopup();
          errorMessage("Request to shutdown bot sent", 4000);
          this.render();
          this.app.finishedTask();
        })
        .catch((e) => {
          this._closePopup();
          this.prpcError(e, "bot/terminate"); // calls app.finishedTask()
          this.render();
        });
    }

    _toggleBotState(e) {
      this._showState = !this._showState;
      this._stateChanged();
      this.render();
    }

    _toggleShowAll(e) {
      // prevent double event
      e.preventDefault();
      this._showAll = !this._showAll;
      this._stateChanged();
      this.render();
    }

    _updateID(e) {
      const idInput = $$("#id_input", this);
      this._botId = idInput.value;
      this._resetCursors();
      this._stateChanged();
      this._fetch();
      this.render();
    }
  }
);
