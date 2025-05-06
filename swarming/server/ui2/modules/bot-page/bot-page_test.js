// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import "./bot-page";
import fetchMock from "fetch-mock";
import { mockUnauthorizedPrpc } from "../test_util";
import { $, $$ } from "common-sk/modules/dom";
import {
  customMatchers,
  expectNoUnmatchedCalls,
  mockUnauthorizedSwarmingService,
  MATCHED,
  mockPrpc,
  eventually,
} from "../test_util";
import { botDataMap, eventsMap, tasksMap } from "./test_data";

describe("bot-page", function () {
  const TEST_BOT_ID = "example-gce-001";

  // "deterministically" set the ordering of the keys in json object.
  // Works for non-nested objects.
  const stringify = (obj) => JSON.stringify(obj, Object.keys(obj).sort());
  const checkFor = (prpcCall, expectedBody) => {
    return (call) => {
      if (call[0].endsWith(prpcCall)) {
        return stringify(JSON.parse(call[1].body)) === stringify(expectedBody);
      }
      return false;
    };
  };
  const mockGetBot = (data) =>
    mockPrpc(fetchMock, "swarming.v2.Bots", "GetBot", data);
  const mockListBotTasks = (data) =>
    mockPrpc(fetchMock, "swarming.v2.Bots", "ListBotTasks", data);
  const mockListBotEvents = (data) =>
    mockPrpc(fetchMock, "swarming.v2.Bots", "ListBotEvents", data);

  beforeEach(function () {
    jasmine.addMatchers(customMatchers);
    // Clear out any query params we might have to not mess with our current state.
    history.pushState(
      null,
      "",
      window.location.origin + window.location.pathname + "?"
    );
  });

  beforeEach(function () {
    // These are the default responses to the expected API calls (aka 'matched').
    // They can be overridden for specific tests, if needed.
    mockUnauthorizedSwarmingService(
      fetchMock,
      {
        cancelTask: false,
      },
      {
        serverVersion:
          "e962671e3bc53f7740f1ceadd04974a3ce94f0e5624e8544770246b5ebf2c46e",
        botVersion:
          "e962671e3bc53f7740f1ceadd04974a3ce94f0e5624e8544770246b5ebf2c46e",
      }
    );

    // By default, don't have any handlers mocked out - this requires
    // tests to opt-in to wanting certain request data.

    // Everything else
    fetchMock.catch(404);
  });

  afterEach(function () {
    // Completely remove the mocking which allows each test
    // to be able to mess with the mocked routes w/o impacting other tests.
    fetchMock.reset();
  });

  // A reusable HTML element in which we create our element under test.
  const container = document.createElement("div");
  document.body.appendChild(container);

  afterEach(function () {
    container.innerHTML = "";
  });

  beforeEach(function () {
    // Fix the time so all of our relative dates work.
    // Note, this turns off the default behavior of setTimeout and related.
    jasmine.clock().install();
    jasmine.clock().mockDate(new Date(Date.UTC(2019, 1, 12, 18, 46, 22, 1234)));
  });

  afterEach(function () {
    jasmine.clock().uninstall();
  });

  // calls the test callback with one element 'ele', a created <bot-page>.
  function createElement(test) {
    return window.customElements.whenDefined("bot-page").then(() => {
      container.innerHTML = `<bot-page testing_offline=true></bot-page>`;
      expect(container.firstElementChild).toBeTruthy();
      test(container.firstElementChild);
    });
  }

  function userLogsIn(ele, callback) {
    // The swarming-app emits the 'busy-end' event when all pending
    // fetches (and renders) have resolved.
    let ran = false;
    ele.addEventListener("busy-end", (e) => {
      if (!ran) {
        ran = true; // prevent multiple runs if the test makes the
        // app go busy (e.g. if it calls fetch).
        callback();
      }
    });
    const login = $$("oauth-login", ele);
    login._logIn();
    fetchMock.flush();
  }

  // convenience function to save indentation and boilerplate.
  // expects a function test that should be called with the created
  // <bot-page> after the user has logged in.
  function loggedInBotPage(test, emptyBotId) {
    createElement((ele) => {
      if (!emptyBotId) {
        ele._botId = TEST_BOT_ID;
      }
      userLogsIn(ele, () => {
        test(ele);
      });
    });
  }

  function serveBot(botName) {
    const bot = botDataMap[botName];
    const events = { items: eventsMap["SkiaGPU"] };
    const tasks = { items: tasksMap["SkiaGPU"] };
    mockPrpc(
      fetchMock,
      "swarming.v2.Swarming",
      "GetPermissions",
      {},
      undefined,
      true
    );
    mockGetBot(bot);
    mockListBotTasks(tasks);
    mockListBotEvents(events);
  }

  // ===============TESTS START====================================

  describe("html structure", function () {
    it("contains swarming-app as its only child", function (done) {
      createElement((ele) => {
        expect(ele.children).toHaveSize(1);
        expect(ele.children[0].tagName).toBe("swarming-app".toUpperCase());
        done();
      });
    });

    describe("when not logged in", function () {
      it("tells the user they should log in", function (done) {
        createElement((ele) => {
          const loginMessage = $$("swarming-app>main .message", ele);
          expect(loginMessage).toBeTruthy();
          expect(loginMessage).not.toHaveAttribute(
            "hidden",
            "Message should not be hidden"
          );
          expect(loginMessage.textContent).toContain("must sign in");
          done();
        });
      });

      it("hides all other elements", function (done) {
        createElement((ele) => {
          // other stuff is hidden
          let content = $("main > *");
          expect(content).toHaveSize(5); // 4 top level sections and a message
          for (const div of content) {
            if (
              div.tagName !== "H2" &&
              div.tagName !== "FLEET-CONSOLE-BANNER"
            ) {
              expect(div).toHaveAttribute("hidden");
            }
          }
          ele._botId = TEST_BOT_ID;
          ele.render();
          // even if an id was given
          content = $("main > *");
          expect(content).toHaveSize(5); // 4 top level sections and a message
          for (const div of content) {
            if (
              div.tagName !== "H2" &&
              div.tagName !== "FLEET-CONSOLE-BANNER"
            ) {
              expect(div).toHaveAttribute("hidden");
            }
          }
          done();
        });
      });
    }); // end describe('when not logged in')

    describe("when logged in as unauthorized user", function () {
      function notAuthorized() {
        mockPrpc(
          fetchMock,
          "swarming.v2.Swarming",
          "GetPermissions",
          {},
          undefined,
          true
        );
        mockUnauthorizedPrpc(
          fetchMock,
          "swarming.v2.Swarming",
          "GetDetails",
          undefined,
          true
        );
        mockUnauthorizedPrpc(fetchMock, "swarming.v2.Bots", "GetBot");
        mockUnauthorizedPrpc(fetchMock, "swarming.v2.Bots", "ListBotTasks");
        mockUnauthorizedPrpc(fetchMock, "swarming.v2.Bots", "ListBotEvents");
      }

      beforeEach(notAuthorized);

      it("tells the user they should change accounts", function (done) {
        loggedInBotPage((ele) => {
          const loginMessage = $$("swarming-app>main .message", ele);
          expect(loginMessage).toBeTruthy();
          expect(loginMessage).not.toHaveAttribute(
            "hidden",
            "Message should not be hidden"
          );
          expect(loginMessage.textContent).toContain("different account");
          done();
        });
      });

      it("does not display logs or task details", function (done) {
        loggedInBotPage((ele) => {
          const content = $$("main .content", ele);
          expect(content).toBeTruthy();
          expect(content).toHaveAttribute("hidden");
          done();
        });
      });
    }); // end describe('when logged in as unauthorized user')

    describe("authorized user, but no bot id", function () {
      it("tells the user they should enter a bot id", function (done) {
        loggedInBotPage((ele) => {
          const loginMessage = $$(".id_buttons .message", ele);
          expect(loginMessage).toBeTruthy();
          expect(loginMessage.textContent).toContain("Enter a Bot ID");
          done();
        }, true);
      });

      it("does not display filters or tasks", function (done) {
        loggedInBotPage((ele) => {
          const content = $$("main .content", ele);
          expect(content).toBeTruthy();
          expect(content).toHaveAttribute("hidden");
          done();
        }, true);
      });
    }); // end describe('authorized user, but no taskid')

    describe("gpu bot with a running task", function () {
      beforeEach(() => serveBot("running"));

      it("renders some of the bot data", function (done) {
        loggedInBotPage((ele) => {
          const dataTable = $$("table.data_table", ele);
          expect(dataTable).toBeTruthy();

          const rows = $("tr", dataTable);
          expect(rows).toBeTruthy();
          expect(rows.length).toBeTruthy();

          // little helper for readability
          const cell = (r, c) => rows[r].children[c];

          const deleteBtn = $$("button.delete", cell(0, 2));
          expect(deleteBtn).toBeTruthy();
          expect(deleteBtn).toHaveClass("hidden");
          const shutDownBtn = $$("button.shut_down", cell(0, 2));
          expect(shutDownBtn).toBeTruthy();
          expect(shutDownBtn).not.toHaveClass("hidden");

          expect(rows[2]).toHaveClass("hidden", "not quarantined");
          expect(rows[3]).toHaveClass("hidden", "not dead");
          expect(rows[4]).toHaveClass("hidden", "not in maintenance");
          expect(cell(5, 0)).toMatchTextContent("Current Task");
          expect(cell(5, 1)).toMatchTextContent("42fb00e06d95be11");
          expect(cell(5, 1).innerHTML).toContain("<a ", "has a link");
          expect(cell(5, 1).innerHTML).toContain(
            'href="/task?id=42fb00e06d95be10"'
          );
          expect(cell(6, 0)).toMatchTextContent("Dimensions");
          expect(cell(11, 0)).toMatchTextContent("gpu");
          expect(cell(11, 1)).toMatchTextContent(
            "NVIDIA (10de) | " +
              "NVIDIA Quadro P400 (10de:1cb3) | NVIDIA Quadro P400 (10de:1cb3-25.21.14.1678)"
          );
          expect(cell(23, 0)).toMatchTextContent("Bot Version");
          expect(rows[23]).toHaveClass("old_version");
          done();
        });
      });

      it("renders the tasks in a table", function (done) {
        loggedInBotPage((ele) => {
          ele._showEvents = false;
          ele.render();
          const tasksTable = $$("table.tasks_table", ele);
          expect(tasksTable).toBeTruthy();

          const rows = $("tr", tasksTable);
          expect(rows).toBeTruthy();
          expect(rows).toHaveSize(1 + 30, "1 for header, 30 tasks");

          // little helper for readability
          const cell = (r, c) => rows[r].children[c];

          // row 0 is the header
          expect(cell(1, 0)).toMatchTextContent(
            "post task for flash build243-m4--device1 to N2G48C"
          );
          expect(cell(1, 0).innerHTML).toContain("<a ", "has a link");
          expect(cell(1, 0).innerHTML).toContain(
            'href="/task?id=61b9da1ebd045410"'
          );
          expect(cell(1, 2)).toMatchTextContent("5.66s");
          expect(cell(1, 3)).toMatchTextContent("SUCCESS");
          expect(rows[1]).not.toHaveClass("pending_task");
          expect(cell(2, 2)).toMatchTextContent("7m 23s");
          expect(cell(2, 3)).toMatchTextContent("SUCCESS");
          expect(rows[2]).not.toHaveClass("pending_task");
          expect(rows[2]).not.toHaveClass("failed_task)");
          expect(rows[2]).not.toHaveClass("exception");
          expect(rows[2]).not.toHaveClass("bot_died");

          const eBtn = $$("main button.more_events", ele);
          expect(eBtn).toBeFalsy();

          const tBtn = $$("main button.more_tasks", ele);
          expect(tBtn).toBeTruthy();
          done();
        });
      });

      it("renders all events in a table", function (done) {
        loggedInBotPage((ele) => {
          ele._showEvents = true;
          ele._showAll = true;
          ele.render();
          const eventsTable = $$("table.events_table", ele);
          expect(eventsTable).toBeTruthy();

          const rows = $("tr", eventsTable);
          expect(rows).toBeTruthy();
          expect(rows).toHaveSize(1 + 50, "1 for header, 50 events");

          // little helper for readability
          const cell = (r, c) => rows[r].children[c];

          // row 0 is the header
          expect(cell(1, 0)).toMatchTextContent("");
          expect(cell(1, 1)).toMatchTextContent("request_task");
          expect(cell(1, 3).innerHTML).toContain("<a ", "has a link");
          expect(cell(1, 3).innerHTML).toContain('href="/task?id=12340"');
          expect(cell(1, 4)).toMatchTextContent("e962671e3b");
          expect(cell(1, 4)).not.toHaveClass("old_version");

          expect(cell(5, 0)).toMatchTextContent(
            "About to restart: Updating to e962671e3bc53f7740f1ceadd04974a3ce94f0e5624e8544770246b5ebf2c46e"
          );
          expect(cell(5, 1)).toMatchTextContent("bot_shutdown");
          expect(cell(5, 3)).toMatchTextContent("");
          expect(cell(5, 4)).toMatchTextContent("f9d34dcc2b");
          expect(cell(5, 4)).toHaveClass("old_version");
          done();
        });
      });

      it("renders some events in a table", function (done) {
        loggedInBotPage((ele) => {
          ele._showEvents = true;
          ele._showAll = false;
          ele.render();
          const eventsTable = $$("table.events_table", ele);
          expect(eventsTable).toBeTruthy();

          const rows = $("tr", eventsTable);
          expect(rows).toBeTruthy();
          expect(rows).toHaveSize(1 + 17, "1 for header, 17 shown events");

          // little helper for readability
          const cell = (r, c) => rows[r].children[c];

          // row 0 is the header
          expect(cell(1, 0)).toMatchTextContent(
            "Rebooting device because max uptime exceeded during idle"
          );
          expect(cell(1, 1)).toMatchTextContent("bot_log");
          expect(cell(1, 3)).toMatchTextContent("");
          expect(cell(1, 4)).toMatchTextContent("e962671e3b");
          expect(cell(1, 4)).not.toHaveClass("old_version");

          const eBtn = $$("main button.more_events", ele);
          expect(eBtn).toBeTruthy();

          const tBtn = $$("main button.more_tasks", ele);
          expect(tBtn).toBeFalsy();
          done();
        });
      });

      it("disables buttons for unprivileged users", function (done) {
        loggedInBotPage((ele) => {
          ele.permissions.cancelTask = false;
          ele.permissions.deleteBot = false;
          ele.permissions.terminateBot = false;
          ele.render();
          const killBtn = $$("main button.kill", ele);
          expect(killBtn).toBeTruthy();
          expect(killBtn).toHaveAttribute("disabled");

          const deleteBtn = $$("main button.delete", ele);
          expect(deleteBtn).toBeTruthy();
          expect(deleteBtn).toHaveAttribute("disabled");

          const tBtn = $$("main button.shut_down", ele);
          expect(tBtn).toBeTruthy();
          expect(tBtn).toHaveAttribute("disabled");

          done();
        });
      });

      it("enables buttons for privileged users", function (done) {
        loggedInBotPage((ele) => {
          ele.permissions.cancelTask = true;
          ele.permissions.deleteBot = true;
          ele.permissions.terminateBot = true;
          ele.render();
          const killBtn = $$("main button.kill", ele);
          expect(killBtn).toBeTruthy();
          expect(killBtn).not.toHaveAttribute("disabled");

          const deleteBtn = $$("main button.delete", ele);
          expect(deleteBtn).toBeTruthy();
          expect(deleteBtn).not.toHaveAttribute("disabled");

          const tBtn = $$("main button.shut_down", ele);
          expect(tBtn).toBeTruthy();
          expect(tBtn).not.toHaveAttribute("disabled");

          done();
        });
      });

      it("does not show android devices section", function (done) {
        loggedInBotPage((ele) => {
          const devTable = $$("table.devices", ele);
          expect(devTable).toBeFalsy();
          done();
        });
      });

      it("has a summary table of the tasks", function (done) {
        loggedInBotPage((ele) => {
          const sTable = $$("bot-page-summary table", ele);
          expect(sTable).toBeTruthy();

          const rows = $("tr", sTable);
          expect(rows).toBeTruthy();
          // header, 8 unique tasks + header and footer
          expect(rows).toHaveSize(10);

          // little helper for readability
          const cell = (r, c) => rows[r].children[c];

          expect(cell(1, 0)).toMatchTextContent(
            "post task for flash build243-m4--device1 to N2G48C"
          );

          expect(cell(5, 0)).toMatchTextContent(
            "Flash build243-m4--device1 to N2G48C"
          );
          expect(cell(5, 1)).toMatchTextContent("4"); // Total
          // TODO(jonahhooper) Fix these aggregates.
          // They are not correctly set.
          expect(cell(5, 2)).toMatchTextContent("0"); // Success
          expect(cell(5, 3)).toMatchTextContent("2"); // Failed
          expect(cell(5, 4)).toMatchTextContent("0"); // Died
          expect(cell(5, 5)).toMatchTextContent("1m 12s"); // duration
          expect(cell(5, 6)).toMatchTextContent("6.83s"); // overhead
          expect(cell(5, 7)).toMatchTextContent("12.3%"); // percent

          done();
        });
      });
    }); // end describe('gpu bot with a running task')

    describe("quarantined android bot", function () {
      beforeEach(() => serveBot("quarantined"));

      it("displays a quarantined message", function (done) {
        loggedInBotPage((ele) => {
          const dataTable = $$("table.data_table", ele);
          expect(dataTable).toBeTruthy();

          const rows = $("tr", dataTable);
          expect(rows).toBeTruthy();
          expect(rows.length).toBeTruthy();

          // little helper for readability
          const cell = (r, c) => rows[r].children[c];

          expect(rows[2]).not.toHaveClass("hidden", "quarantined");
          expect(cell(2, 1)).toMatchTextContent("No available devices.");
          expect(rows[3]).toHaveClass("hidden", "not dead");
          expect(rows[4]).toHaveClass("hidden", "not in maintenance");
          expect(cell(5, 0)).toMatchTextContent("Current Task");
          expect(cell(5, 1)).toMatchTextContent("idle");

          done();
        });
      });

      it("shows android devices section", function (done) {
        loggedInBotPage((ele) => {
          const devTable = $$("table.devices", ele);
          expect(devTable).toBeTruthy();

          const rows = $("tr", devTable);
          expect(rows).toBeTruthy();
          expect(rows).toHaveSize(2); // 1 for header, 1 device

          // little helper for readability
          const cell = (r, c) => rows[r].children[c];

          expect(cell(1, 0)).toMatchTextContent("3BE9F057");
          expect(cell(1, 1)).toMatchTextContent("100");
          expect(cell(1, 2)).toMatchTextContent("???");
          expect(cell(1, 3)).toMatchTextContent(
            "still booting (sys.boot_completed)"
          );

          done();
        });
      });
    }); // describe('quarantined android bot')

    describe("dead bot", function () {
      beforeEach(() => serveBot("dead"));

      it("displays buttons and table items indicating bot death", function (done) {
        loggedInBotPage((ele) => {
          const dataTable = $$("table.data_table", ele);
          expect(dataTable).toBeTruthy();

          const rows = $("tr", dataTable);
          expect(rows).toBeTruthy();
          expect(rows.length).toBeTruthy();

          // little helper for readability
          const cell = (r, c) => rows[r].children[c];

          const deleteBtn = $$("button.delete", cell(0, 2));
          expect(deleteBtn).toBeTruthy();
          expect(deleteBtn).not.toHaveClass("hidden");
          const shutDownBtn = $$("button.shut_down", cell(0, 2));
          expect(shutDownBtn).toBeTruthy();
          expect(shutDownBtn).toHaveClass("hidden");

          expect(rows[1]).toHaveClass("dead");
          expect(rows[2]).toHaveClass("hidden", "not quarantined");
          expect(rows[3]).not.toHaveClass("hidden", "dead");
          expect(rows[4]).toHaveClass("hidden", "not in maintenance");
          expect(cell(5, 0)).toMatchTextContent("Died on Task");
          done();
        });
      });

      it("does not display kill task on dead bot", function (done) {
        loggedInBotPage((ele) => {
          ele._bot.taskId = "t1233";
          const dataTable = $$("table.data_table", ele);
          expect(dataTable).toBeTruthy();

          const rows = $("tr", dataTable);
          expect(rows).toBeTruthy();
          expect(rows.length).toBeTruthy();

          // little helper for readability
          const cell = (r, c) => rows[r].children[c];

          const killBtn = $$("button.kill", cell(5, 2));
          expect(killBtn).toBeTruthy();
          expect(killBtn).toHaveAttribute("hidden");

          done();
        });
      });
    }); // describe('dead machine provider bot')
  }); // end describe('html structure')

  describe("dynamic behavior", function () {
    it("hides and unhides extra details with a button", function (done) {
      serveBot("running");
      loggedInBotPage((ele) => {
        ele._showState = false;
        ele.render();

        const state = $$(".bot_state", ele);
        expect(state).toBeTruthy();
        expect(state).toHaveAttribute("hidden");

        const stateBtn = $$("button.state", ele);
        expect(stateBtn).toBeTruthy();

        stateBtn.click();

        expect(state).not.toHaveAttribute("hidden");

        stateBtn.click();
        expect(state).toHaveAttribute("hidden");

        done();
      });
    });
  }); // end describe('dynamic behavior')

  describe("api calls", function () {
    it("makes no API calls when not logged in", function (done) {
      createElement((ele) => {
        fetchMock.flush(true).then(() => {
          // MATCHED calls are calls that we expect and specified in the
          // beforeEach at the top of this file.
          let calls = fetchMock.calls(MATCHED, "GET");
          expect(calls).toHaveSize(0);
          calls = fetchMock.calls(MATCHED, "POST");
          expect(calls).toHaveSize(0);

          expectNoUnmatchedCalls(fetchMock);
          done();
        });
      });
    });

    function checkAuthorization(calls) {
      // check authorization headers are set
      calls.forEach((c) => {
        expect(c[1].headers).toBeDefined();
        expect(c[1].headers.authorization).toContain("Bearer ");
      });

      expectNoUnmatchedCalls(fetchMock);
    }

    it("makes auth'd API calls when a logged in user views landing page", function (done) {
      serveBot("running");
      loggedInBotPage((ele) => {
        const prpcCalls = fetchMock.calls(MATCHED, "POST");
        expect(prpcCalls).toHaveSize(
          2 + 1 + 3,
          "2 from swarming-app, 1 for permissions, 3 for events, tasks and botInfo"
        );

        const getBotsReq = { botId: TEST_BOT_ID };
        const getBotsCall = prpcCalls.filter(
          checkFor("prpc/swarming.v2.Bots/GetBot", getBotsReq)
        );
        expect(getBotsCall).toHaveSize(
          1,
          `Must have one GetBot request with the value: ${stringify(
            getBotsReq
          )}`
        );

        const listBotTasksReq = {
          botId: TEST_BOT_ID,
          cursor: "",
          includePerformanceStats: true,
          limit: 30,
          sort: "QUERY_STARTED_TS",
          state: "QUERY_ALL",
        };
        const listBotsCall = prpcCalls.filter(
          checkFor("prpc/swarming.v2.Bots/ListBotTasks", listBotTasksReq)
        );
        expect(listBotsCall).toHaveSize(
          1,
          `Must have one ListBotTasks request with the value: ${stringify(
            listBotTasksReq
          )}`
        );

        const listBotEventsReq = {
          botId: TEST_BOT_ID,
          cursor: "",
          limit: 50,
        };
        const listBotEvents = prpcCalls.filter(
          checkFor("prpc/swarming.v2.Bots/ListBotEvents", listBotEventsReq)
        );
        expect(listBotEvents).toHaveSize(
          1,
          `Must have one ListBotEvents request with the value: ${stringify(
            listBotEventsReq
          )}`
        );

        checkAuthorization(prpcCalls);

        done();
      });
    });

    it("can kill a running task", function (done) {
      serveBot("running");
      loggedInBotPage((ele) => {
        ele.permissions.cancelTask = true;
        ele.render();
        fetchMock.resetHistory();
        // This is the taskId on the 'running' bot.
        mockPrpc(fetchMock, "swarming.v2.Tasks", "CancelTask", {
          canceled: true,
          wasRunning: true,
        });

        const killBtn = $$("main button.kill", ele);
        expect(killBtn).toBeTruthy();

        killBtn.click();

        const dialog = $$(".prompt-dialog", ele);
        expect(dialog).toBeTruthy();
        expect(dialog).toHaveClass("opened");

        const okBtn = $$("button.ok", dialog);
        expect(okBtn).toBeTruthy();

        okBtn.click();

        fetchMock.flush(true).then(() => {
          // MATCHED calls are calls that we expect and specified in the
          // beforeEach at the top of this file.
          expectNoUnmatchedCalls(fetchMock);
          let calls = fetchMock.calls(MATCHED, "GET");
          expect(calls).toHaveSize(0);
          calls = fetchMock.calls(MATCHED, "POST");
          expect(calls).toHaveSize(1);
          const call = calls[0];
          const options = call[1];
          expect(options.body).toContain('"killRunning":true');

          done();
        });
      });
    });

    it("can terminate a non-dead bot", function (done) {
      serveBot("running");
      loggedInBotPage((ele) => {
        ele.permissions.terminateBot = true;
        ele.render();
        fetchMock.resetHistory();
        // This is the taskId on the 'running' bot.
        mockPrpc(fetchMock, "swarming.v2.Bots", "TerminateBot", {
          taskId: "some_task_id",
        });

        const tBtn = $$("main button.shut_down", ele);
        expect(tBtn).toBeTruthy();

        tBtn.click();

        const dialog = $$(".prompt-dialog", ele);
        expect(dialog).toBeTruthy();
        expect(dialog).toHaveClass("opened");

        const reason = "Bot misbehaving";
        const reasonInput = $$("#reason", ele);
        expect(reasonInput).toBeTruthy();
        reasonInput.value = reason;

        const okBtn = $$("button.ok", dialog);
        expect(okBtn).toBeTruthy();

        okBtn.click();

        fetchMock.flush(true).then(() => {
          // MATCHED calls are calls that we expect and specified in the
          // beforeEach at the top of this file.
          expectNoUnmatchedCalls(fetchMock);
          let calls = fetchMock.calls(MATCHED, "GET");
          expect(calls).toHaveSize(0);
          calls = fetchMock.calls(MATCHED, "POST").filter(
            checkFor("swarming.v2.Bots/TerminateBot", {
              botId: TEST_BOT_ID,
              reason: reason,
            })
          );
          expect(calls).toHaveSize(1);

          done();
        });
      });
    });

    it("can delete a dead bot", function (done) {
      serveBot("dead");
      loggedInBotPage((ele) => {
        ele.permissions.deleteBot = true;
        ele.render();
        fetchMock.resetHistory();
        // This is the taskId on the 'running' bot.
        mockPrpc(fetchMock, "swarming.v2.Bots", "DeleteBot", {});

        const deleteBtn = $$("main button.delete", ele);
        expect(deleteBtn).toBeTruthy();

        deleteBtn.click();

        const dialog = $$(".prompt-dialog", ele);
        expect(dialog).toBeTruthy();
        expect(dialog).toHaveClass("opened");

        const okBtn = $$("button.ok", dialog);
        expect(okBtn).toBeTruthy();

        okBtn.click();

        fetchMock.flush(true).then(() => {
          // MATCHED calls are calls that we expect and specified in the
          // beforeEach at the top of this file.
          expectNoUnmatchedCalls(fetchMock);
          let calls = fetchMock.calls(MATCHED, "GET");
          expect(calls).toHaveSize(0);
          calls = fetchMock.calls(MATCHED, "POST");
          expect(calls).toHaveSize(1);

          done();
        });
      });
    });

    it("can fetch more tasks", function (done) {
      serveBot("running");
      loggedInBotPage((ele) => {
        ele._taskCursor = "myCursor";
        ele._showEvents = false;
        ele.render();
        fetchMock.reset(); // clears history and routes

        const data = tasksMap["SkiaGPU"];
        mockListBotTasks({
          items: data,
          cursor: "newCursor",
        });

        const tBtn = $$("main button.more_tasks", ele);
        expect(tBtn).toBeTruthy();

        tBtn.click();

        fetchMock.flush(true).then(() => {
          // MATCHED calls are calls that we expect and specified in the
          // beforeEach at the top of this file.
          expectNoUnmatchedCalls(fetchMock);
          const calls = fetchMock.calls(MATCHED, "POST");
          expect(calls).toHaveSize(1);

          const req = JSON.parse(calls[0][1].body);
          // spot check a few fields
          expect(req.state).toEqual("QUERY_ALL");
          expect(req.botId).toEqual(TEST_BOT_ID);
          expect(req.limit).toEqual(30);
          // validate cursor
          // Hack bc prpc causes the fetch to terminate early.
          // This will cause fetchMock.flush to exit before rendering has been
          // completed for the tBtn.click() action.
          // Add additional event listener to wait for all rendering to complete
          // before running tests on the final state of UI.
          eventually(ele, (ele) => {
            expect(req.cursor).toEqual("myCursor");
            expect(ele._taskCursor).toEqual(
              "newCursor",
              "cursor should update"
            );
            expect(ele._tasks).toHaveSize(
              30 + 30,
              "30 initial tasks, 30 new tasks"
            );
            done();
          });
        });
      });
    });

    it("can fetch more events", function (done) {
      serveBot("running");
      loggedInBotPage((ele) => {
        ele._eventsCursor = "myCursor";
        ele._showEvents = true;
        ele.render();
        fetchMock.reset(); // clears history and routes

        mockListBotEvents({
          items: eventsMap["SkiaGPU"],
          cursor: "newCursor",
        });

        const eBtn = $$("main button.more_events", ele);
        expect(eBtn).toBeTruthy();

        eBtn.click();

        fetchMock.flush(true).then(() => {
          // MATCHED calls are calls that we expect and specified in the
          // beforeEach at the top of this file.
          expectNoUnmatchedCalls(fetchMock);
          const calls = fetchMock.calls(MATCHED, "POST");
          expect(calls).toHaveSize(1);

          const request = JSON.parse(calls[0][1].body);
          // spot check a few fields
          expect(request.botId).toEqual(TEST_BOT_ID);
          expect(request.limit).toEqual(50);
          // validate cursor
          expect(request.cursor).toEqual("myCursor");
          eventually(ele, (ele) => {
            expect(ele._eventsCursor).toEqual(
              "newCursor",
              "cursor should update"
            );
            expect(ele._events).toHaveSize(
              50 + 50,
              "50 initial tasks, 50 new tasks"
            );
          });

          done();
        });
      });
    });

    it("reloads tasks and events on refresh", function (done) {
      serveBot("running");
      loggedInBotPage((ele) => {
        ele.render();
        fetchMock.reset(); // clears history and routes

        serveBot("running");

        const rBtn = $$("main button.refresh", ele);
        expect(rBtn).toBeTruthy();

        rBtn.click();

        fetchMock.flush(true).then(() => {
          // MATCHED calls are calls that we expect and specified in the
          // beforeEach at the top of this file.
          expectNoUnmatchedCalls(fetchMock);
          const prpcCalls = fetchMock.calls(MATCHED, "POST");
          expect(prpcCalls).toHaveSize(4);

          done();
        });
      });
    });
  }); // end describe('api calls')
});
