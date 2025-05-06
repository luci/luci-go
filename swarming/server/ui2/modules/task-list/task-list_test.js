// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import "./task-list";
import fetchMock from "fetch-mock";
import { deepEquals, mockUnauthorizedPrpc } from "../test_util";
import { convertFromLegacyState, Timestamp } from "./task-list-helpers";
import { deepCopy } from "common-sk/modules/object";
import { $, $$ } from "common-sk/modules/dom";
import {
  childrenAsArray,
  customMatchers,
  expectNoUnmatchedCalls,
  getChildItemWithText,
  mockUnauthorizedSwarmingService,
  MATCHED,
  mockPrpc,
  eventually,
} from "../test_util";
import {
  column,
  filterTasks,
  getColHeader,
  listQueryParams,
  processTasks,
} from "./task-list-helpers";
import { tasks22, fleetDimensions } from "./test_data";

describe("task-list", function () {
  beforeEach(function () {
    jasmine.addMatchers(customMatchers);
    // Clear out any query params we might have to not mess with our current state.
    history.pushState(
      null,
      "",
      window.location.origin + window.location.pathname + "?"
    );
  });

  const mockOutRpcsWithFetchMock = (excludeListTasks = false) => {
    // These are the default responses to the expected API calls (aka 'matched').
    // They can be overridden for specific tests, if needed.
    mockUnauthorizedSwarmingService(fetchMock, {});
    fetchMock.get("glob:/_ah/api/swarming/v1/server/permissions*", {
      cancel_task: false,
    });

    if (!excludeListTasks) {
      mockPrpc(fetchMock, "swarming.v2.Tasks", "ListTasks", tasks22);
    }
    mockPrpc(
      fetchMock,
      "swarming.v2.Bots",
      "GetBotDimensions",
      fleetDimensions
    );
    mockPrpc(fetchMock, "swarming.v2.Tasks", "CountTasks", { count: 12345 });

    // Everything else
    fetchMock.catch(404);
  };

  beforeEach(function () {
    mockOutRpcsWithFetchMock();
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
  const now = Date.UTC(2018, 11, 19, 16, 46, 22, 1234);
  const yesterday = new Date(now - 24 * 60 * 60 * 1000);
  beforeEach(function () {
    // Fix the time so all of our relative dates work.
    // Note, this turns off the default behavior of setTimeout and related.
    jasmine.clock().install();
    jasmine.clock().mockDate(new Date(now));
  });

  afterEach(function () {
    jasmine.clock().uninstall();
  });

  // calls the test callback with one element 'ele', a created <swarming-index>.
  // We can't put the describes inside the whenDefined callback because
  // that doesn't work on Firefox (and possibly other places).
  function createElement(test) {
    return window.customElements.whenDefined("task-list").then(() => {
      container.innerHTML = `<task-list testing_offline=true></task-list>`;
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
  // <task-list> after the user has logged in.
  function loggedInTasklist(test) {
    createElement((ele) => {
      userLogsIn(ele, () => {
        test(ele);
      });
    });
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
      it("does not display filters or tasks", function (done) {
        createElement((ele) => {
          const taskTable = $$(".task-table", ele);
          expect(taskTable).toBeTruthy();
          expect(taskTable).toHaveAttribute(
            "hidden",
            ".task-table should be hidden"
          );
          expect($$(".header", ele)).toHaveAttribute(
            "hidden",
            "no filters seen"
          );
          done();
        });
      });
    }); // end describe('when not logged in')

    describe("when logged in as unauthorized user", function () {
      function notAuthorized() {
        // overwrite the default fetchMock behaviors to have everything return 403.
        mockPrpc(
          fetchMock,
          "swarming.v2.Swarming",
          "GetPermissions",
          { listTasks: ["pool1"] },
          undefined,
          true
        );
        mockUnauthorizedPrpc(fetchMock, "swarming.v2.Swarming", "GetDetails");
        mockUnauthorizedPrpc(fetchMock, "swarming.v2.Tasks", "ListTasks");
        mockUnauthorizedPrpc(fetchMock, "swarming.v2.Bots", "GetBotDimensions");
        mockUnauthorizedPrpc(fetchMock, "swarming.v2.Tasks", "CountTasks");
      }

      beforeEach(notAuthorized);

      it("displays only pool filters", function (done) {
        loggedInTasklist((ele) => {
          const taskTable = $(".task-table", ele);
          expect(taskTable).toBeTruthy();

          const taskRows = $(".task-table .task-row", ele);
          expect(taskRows).toHaveSize(0);

          const keyFilters = $(".filter_box .selector.keys .item", ele);
          expect(keyFilters).toHaveSize(1);
          expect(keyFilters[0]).toMatchTextContent("pool (tag)");

          // click 'pool' filter.
          keyFilters[0].click();
          const valueFilters = $(".filter_box .selector.values .item", ele);
          expect(valueFilters).toHaveSize(1);
          expect(valueFilters[0]).toMatchTextContent("pool1");

          // We expect that the filters will be loaded with a specific pool
          // even if the user is unauthorized.
          // Therefore we should ignore all unauthorized errors.
          expect(ele._message).not.toContain("User unauthorized");

          done();
        });
      });
    }); // end describe('when logged in as unauthorized user')

    describe("when logged in as user (not admin)", function () {
      describe("default landing page", function () {
        it("displays whatever tasks show up", function (done) {
          loggedInTasklist((ele) => {
            const rows = $(".task-table .task-row", ele);
            expect(rows).toBeTruthy();
            expect(rows).toHaveSize(22, "(num taskRows)");
            done();
          });
        });

        it("shows the default set of columns", function (done) {
          loggedInTasklist((ele) => {
            // ensure sorting is deterministic.
            ele._sort = "created_ts";
            ele._dir = "desc";
            ele._verbose = false;
            ele.render();

            const colHeaders = $(".task-table thead th", ele);
            expect(colHeaders).toBeTruthy();
            expect(colHeaders).toHaveSize(7, "(num colHeaders)");
            expect(colHeaders[0].innerHTML).toContain("<more-vert-icon-sk");
            expect(colHeaders[0]).toMatchTextContent("Task Name");
            expect(colHeaders[1]).toMatchTextContent("Created On");
            expect(colHeaders[2]).toMatchTextContent("Time Spent Pending");
            expect(colHeaders[3]).toMatchTextContent("Duration");
            expect(colHeaders[4]).toMatchTextContent("Bot Assigned");
            expect(colHeaders[5]).toMatchTextContent("pool (tag)");
            expect(colHeaders[6]).toMatchTextContent("state (of task)");

            const rows = $(".task-table .task-row", ele);
            expect(rows).toBeTruthy();
            expect(rows).toHaveSize(22, "22 rows");

            const cells = $(".task-table .task-row td", ele);
            expect(cells).toBeTruthy();
            expect(cells).toHaveSize(7 * 22, "7 columns * 22 rows");
            // little helper for readability
            const cell = (r, c) => cells[7 * r + c];
            expect(rows[0]).toHaveClass("failed_task");
            expect(rows[0]).not.toHaveClass("exception");
            expect(rows[0]).not.toHaveClass("pending_task");
            expect(rows[0]).not.toHaveClass("bot_died");
            expect(cell(0, 0)).toMatchTextContent(
              "Build-Win-Clang-x86_64-Debug-ANGLE"
            );
            expect(cell(0, 0).innerHTML).toContain("<a ", "has a link");
            expect(cell(0, 0).innerHTML).toContain(
              'href="/task?id=41e031b2c8b46710"',
              "link is correct"
            );
            expect(cell(0, 2)).toMatchTextContent("2.36s"); // pending
            expect(cell(0, 4)).toMatchTextContent("skia-gce-610");
            expect(cell(0, 4).innerHTML).toContain("<a ", "has a link");
            expect(cell(0, 4).innerHTML).toContain(
              'href="/bot?id=skia-gce-610"',
              "link is correct"
            );
            expect(cell(0, 5)).toMatchTextContent("Skia");
            expect(cell(0, 6)).toMatchTextContent("COMPLETED (FAILURE)");

            expect(rows[2]).not.toHaveClass("failed_task");
            expect(rows[2]).not.toHaveClass("exception");
            expect(rows[2]).not.toHaveClass("pending_task");
            expect(rows[2]).not.toHaveClass("bot_died");
            expect(cell(2, 2)).toMatchTextContent("--"); // pending

            expect(rows[4]).not.toHaveClass("failed_task");
            expect(rows[4]).not.toHaveClass("exception");
            expect(rows[4]).not.toHaveClass("pending_task");
            expect(rows[4]).not.toHaveClass("bot_died");
            expect(rows[4]).toHaveClass("client_error");

            expect(rows[5]).not.toHaveClass("failed_task");
            expect(rows[5]).toHaveClass("exception");
            expect(rows[5]).not.toHaveClass("pending_task");
            expect(rows[5]).not.toHaveClass("bot_died");
            expect(cell(5, 3).textContent).toContain("2m  1s");

            expect(rows[6]).not.toHaveClass("failed_task");
            expect(rows[6]).not.toHaveClass("exception");
            expect(rows[6]).toHaveClass("pending_task");
            expect(rows[6]).not.toHaveClass("bot_died");
            expect(cell(6, 3).textContent).toContain("12m 54s*"); // duration*/

            expect(rows[14]).not.toHaveClass("failed_task");
            expect(rows[14]).toHaveClass("exception");
            expect(rows[14]).not.toHaveClass("pending_task");
            expect(rows[14]).not.toHaveClass("bot_died");

            done();
          });
        });
        // TruncateToMinute passes either a string or date
        // date constructor, sets seconds/mills to 0, and
        // returns an int that's seconds since the epoch.
        const truncateToMinute = function (date) {
          const value = new Date(date);
          value.setSeconds(0);
          value.setMilliseconds(0);
          return value.getTime() / 1000;
        };
        it("supplies past 24 hours for the time pickers", function (done) {
          loggedInTasklist((ele) => {
            const start = $$("#start_time", ele);
            expect(start).toBeTruthy();
            expect(start.disabled).toBeFalsy();
            expect(truncateToMinute(start.value)).toBe(
              truncateToMinute(yesterday),
              "(start time is 24 hours ago)"
            );

            const end = $$("#end_time", ele);
            expect(end).toBeTruthy();
            expect(end.disabled).toBeTruthy();
            expect(truncateToMinute(end.value)).toBe(
              truncateToMinute(now),
              "(end time is now)"
            );

            const checkbox = $$(".picker checkbox-sk", ele);
            expect(checkbox).toBeTruthy();
            expect(checkbox.checked).toBeTruthy(); // defaults to using now
            done();
          });
        });

        it("shows the counts of the first 7 states", function (done) {
          loggedInTasklist((ele) => {
            ele.render();

            const countRows = $("#query_counts tr", ele);
            expect(countRows).toBeTruthy();
            expect(countRows).toHaveSize(
              1 + 7,
              "(num counts, displayed + 7 states)"
            );

            expect(countRows[0]).toMatchTextContent("Displayed: 22");

            expect(countRows[3].innerHTML).toContain("<a ", "contains a link");
            const link = $$("a", countRows[3]);
            expect(link.href).toContain(
              "at=false" +
                "&c=name&c=createdTs&c=pendingTime" +
                "&c=duration&c=bot&c=pool-tag" +
                "&c=state&d=desc" +
                "&et=1545237983234" +
                "&f=state%3ACOMPLETED_FAILURE" +
                "&k=&n=true&s=createdTs" +
                "&st=1545151583234&v=false"
            );

            // The true on flush waits for res.json() to resolve too
            fetchMock.flush(true).then(() => {
              expect(countRows[5]).toMatchTextContent("Running: 12345");
              done();
            });
          });
        });

        it("shows aliases on filter chips", function (done) {
          loggedInTasklist((ele) => {
            ele._filters = [
              "cpu-tag:x86-64-Haswell_GCE",
              "gpu-tag:10de:1cb3-415.27",
              "device_type-tag:flo",
            ];
            ele.render();

            const chips = $(".chip_container .chip", ele);
            expect(chips).toBeTruthy();
            expect(chips).toHaveSize(3, "3 filters, 3 chips");

            // They are displayed in order, so check content
            expect(chips[0]).toMatchTextContent("cpu-tag:x86-64-Haswell_GCE");
            expect(chips[1]).toMatchTextContent(
              "gpu-tag:NVIDIA Quadro P400 (10de:1cb3-415.27)"
            );
            expect(chips[2]).toMatchTextContent(
              "device_type-tag:Nexus 7 [2013] (flo)"
            );
            done();
          });
        });
      }); // end describe('default landing page')
    }); // end describe('when logged in as user')
  }); // end describe('html structure')

  describe("dynamic behavior", function () {
    it("updates the sort-toggles based on the current sort direction", function (done) {
      loggedInTasklist((ele) => {
        ele._sort = "name";
        ele._dir = "desc";
        ele.render();

        const sortToggles = $(".task-table thead sort-toggle", ele);
        expect(sortToggles).toBeTruthy();
        expect(sortToggles).toHaveSize(7, "(num sort-toggles)");

        expect(sortToggles[0].key).toBe("name");
        expect(sortToggles[0].currentKey).toBe("name");
        expect(sortToggles[0].direction).toBe("desc");
        // spot check one of the other ones
        expect(sortToggles[5].key).toBe("pool-tag");
        expect(sortToggles[5].currentKey).toBe("name");
        expect(sortToggles[5].direction).toBe("desc");

        ele._sort = "createdTs";
        ele._dir = "asc";
        ele.render();

        expect(sortToggles[0].key).toBe("name");
        expect(sortToggles[0].currentKey).toBe("createdTs");
        expect(sortToggles[0].direction).toBe("asc");

        expect(sortToggles[1].key).toBe("createdTs");
        expect(sortToggles[1].currentKey).toBe("createdTs");
        expect(sortToggles[1].direction).toBe("asc");
        done();
      });
    });
    // This is done w/o interacting with the sort-toggles because that is more
    // complicated with adding the event listener and so on.
    it("can stable sort", function (done) {
      loggedInTasklist((ele) => {
        ele._verbose = false;
        // First sort in descending created_ts order
        ele._sort = "created_ts";
        ele._dir = "desc";
        ele.render();
        // next sort in ascending pool-tag
        ele._sort = "pool-tag";
        ele._dir = "asc";
        ele.render();

        const actualIDOrder = ele._tasks.map((t) => t.taskId);
        const actualPoolOrder = ele._tasks.map((t) =>
          column("pool-tag", t, ele)
        );
        expect(actualIDOrder).toEqual([
          "41e0284bc3ef4f10",
          "41e023035ecced10",
          "41e0222076a33010",
          "41e020504d0a5110",
          "41e0204f39d06210",
          "41e01fe02b981410",
          "41dfffb4970ae410",
          "41e0284bf01aef10",
          "41e0222290be8110",
          "41e031b2c8b46710",
          "41dfffb8b1414b10",
          "41dfa79d3bf29010",
          "41df677202f20310",
          "41e04320743a3840",
          "41e020504d0a5119",
          "41e019d8b7aa2f10",
          "41e015d550464910",
          "41e0310fe0b7c410",
          "41e0182a00fcc110",
          "41e016dc85735b10",
          "41dd3d950bb52710",
          "41dd3d9564402e10",
        ]);
        expect(actualPoolOrder).toEqual([
          "Chrome",
          "Chrome",
          "Chrome",
          "Chrome",
          "Chrome",
          "Chrome",
          "Chrome",
          "Chrome-CrOS-VM",
          "Chrome-GPU",
          "Skia",
          "Skia",
          "Skia",
          "Skia",
          "chromium",
          "chromium.tests",
          "fuchsia.tests",
          "fuchsia.tests",
          "luci.chromium.ci",
          "luci.chromium.ci",
          "luci.chromium.ci",
          "luci.fuchsia.try",
          "luci.fuchsia.try",
        ]);
        done();
      });
    });

    it("can sort durations correctly", function (done) {
      loggedInTasklist((ele) => {
        ele._verbose = false;
        ele._sort = "duration";
        ele._dir = "asc";
        ele.render();

        const actualDurationsOrder = ele._tasks.map((t) =>
          t.humanDuration.trim()
        );
        expect(actualDurationsOrder).toEqual([
          "0.62s",
          "2.90s",
          "17.84s",
          "1m 38s",
          "2m  1s",
          "2m  1s",
          "12m 54s*",
          "12m 55s*",
          "1h  9m 47s",
          "2h 16m 15s",
          "--",
          "--",
          "--",
          "--",
          "--",
          "--",
          "--",
          "--",
          "--",
          "--",
          "--",
          "--",
        ]);

        ele._verbose = false;
        ele._sort = "pendingTime";
        ele._dir = "asc";
        ele.render();

        const actualPendingOrder = ele._tasks.map((t) =>
          t.humanPendingTime.trim()
        );
        expect(actualPendingOrder).toEqual([
          "0s",
          "0s",
          "0.63s",
          "0.66s",
          "0.72s",
          "2.35s",
          "2.36s",
          "2.58s",
          "5.74s",
          "8.21s",
          "24.58s",
          "1m 11s",
          "1m 17s",
          "5m  5s",
          "5m 36s",
          "8m  2s*",
          "11m 28s",
          "14m 54s*",
          "14m 55s*",
          "27m 55s",
          "--",
          "--",
        ]);
        done();
      });
    });

    it('toggles columns by clicking on the boxes in the "column selector"', function (done) {
      loggedInTasklist((ele) => {
        ele._cols = ["name"];
        ele._showColSelector = true;
        ele.render();

        const keySelector = $$(".col_selector", ele);
        expect(keySelector).toBeTruthy();

        // click on first non checked checkbox.
        let keyToClick = null;
        let checkbox = null;
        for (let i = 0; i < keySelector.children.length; i++) {
          const child = keySelector.children[i];
          checkbox = $$("checkbox-sk", child);
          if (checkbox && !checkbox.checked) {
            keyToClick = checkbox.getAttribute("data-key");
            break;
          }
        }
        checkbox.click(); // click is synchronous, it returns after
        // the clickHandler is run.
        // Check underlying data
        expect(ele._cols).toContain(keyToClick);
        // check the rendering changed
        let colHeaders = $(".task-table thead th");
        expect(colHeaders).toBeTruthy();
        expect(colHeaders).toHaveSize(2, "(num colHeaders)");
        const expectedHeader = getColHeader(keyToClick);
        expect(colHeaders.map((c) => c.textContent.trim())).toContain(
          expectedHeader
        );

        // We have to find the checkbox again because the order
        // shuffles to keep selected ones on top.
        checkbox = null;
        for (let i = 0; i < keySelector.children.length; i++) {
          const child = keySelector.children[i];
          const key = $$(".key", child);
          if (key && key.textContent.trim() === getColHeader(keyToClick)) {
            checkbox = $$("checkbox-sk", child);
            break;
          }
        }
        expect(checkbox).toBeTruthy(
          "We expected to find a checkbox with header " +
            getColHeader(keyToClick)
        );

        // click it again
        checkbox.click();

        // Check underlying data
        expect(ele._cols).not.toContain(keyToClick);
        // check the rendering changed
        colHeaders = $(".task-table thead th");
        expect(colHeaders).toBeTruthy();
        expect(colHeaders).toHaveSize(1, "(num colHeaders)");
        expect(colHeaders.map((c) => c.textContent.trim())).not.toContain(
          expectedHeader
        );
        done();
      });
    });

    it("shows values when a key row is selected", function (done) {
      loggedInTasklist((ele) => {
        ele._cols = ["name"];
        ele.render();
        let row = getChildItemWithText(
          $$(".selector.keys"),
          "device_type (tag)",
          ele
        );
        expect(row).toBeTruthy();
        row.click();
        expect(row.hasAttribute("selected")).toBeTruthy();
        expect(ele._primaryKey).toBe("device_type-tag");

        let valueSelector = $$(".selector.values");
        expect(valueSelector).toBeTruthy();
        let values = childrenAsArray(valueSelector).map((c) =>
          c.textContent.trim()
        );
        // spot check
        expect(values).toHaveSize(15);
        expect(values).toContain("Nexus 9 (flounder)");
        expect(values).toContain("iPhone X");

        const oldRow = row;
        row = getChildItemWithText(
          $$(".selector.keys"),
          "state (of task)",
          ele
        );
        expect(row).toBeTruthy();
        row.click();
        expect(row.hasAttribute("selected")).toBeTruthy(
          "new row only one selected"
        );
        expect(oldRow.hasAttribute("selected")).toBeFalsy("old row unselected");
        expect(ele._primaryKey).toBe("state");

        valueSelector = $$(".selector.values");
        expect(valueSelector).toBeTruthy();
        values = childrenAsArray(valueSelector).map((c) =>
          c.textContent.trim()
        );
        // spot check
        expect(values).toHaveSize(15);
        expect(values).toContain("RUNNING");
        expect(values).toContain("COMPLETED_FAILURE");
        expect(values).toContain("CLIENT_ERROR");

        done();
      });
    });

    it("orders columns in selector alphabetically with selected cols on top", function (done) {
      loggedInTasklist((ele) => {
        ele._cols = ["duration", "createdTs", "state", "name"];
        ele._showColSelector = true;
        ele._refilterPossibleColumns(); // also calls render

        const keySelector = $$(".col_selector");
        expect(keySelector).toBeTruthy();
        const keys = childrenAsArray(keySelector).map((c) =>
          c.textContent.trim()
        );

        // Skip the first child, which is the input box
        expect(keys.slice(1, 7)).toEqual([
          "Task Name",
          "Created On",
          "Duration",
          "state (of task)",
          "Abandoned On",
          "allow_milo (tag)",
        ]);

        done();
      });
    });

    it("adds a filter when the addIcon is clicked", function (done) {
      loggedInTasklist((ele) => {
        ele._cols = ["duration", "created_ts", "state", "name"];
        ele._primaryKey = "state"; // set 'os' selected
        ele._filters = []; // no filters
        ele.render();

        const valueRow = getChildItemWithText(
          $$(".selector.values"),
          "BOT_DIED",
          ele
        );
        const addIcon = $$("add-circle-icon-sk", valueRow);
        expect(addIcon).toBeTruthy("there should be an icon for adding");
        addIcon.click();

        expect(ele._filters).toHaveSize(1, "a filter should be added");
        expect(ele._filters[0]).toEqual("state:BOT_DIED");

        const chipContainer = $$(".chip_container", ele);
        expect(chipContainer).toBeTruthy(
          "there should be a filter chip container"
        );
        expect(chipContainer.children).toHaveSize(1);
        expect(addIcon.hasAttribute("hidden")).toBeTruthy(
          "addIcon should go away after being clicked"
        );
        done();
      });
    });

    it("removes a filter when the circle clicked", function (done) {
      loggedInTasklist((ele) => {
        ele._cols = ["duration", "created_ts", "state", "name"];
        ele._primaryKey = "pool-tag";
        ele._filters = ["pool-tag:Skia", "state:BOT_DIED"];
        ele.render();

        const filterChip = getChildItemWithText(
          $$(".chip_container"),
          "pool-tag:Skia",
          ele
        );
        const icon = $$("cancel-icon-sk", filterChip);
        expect(icon).toBeTruthy("there should be a icon to remove it");
        icon.click();

        expect(ele._filters).toHaveSize(1, "a filter should be removed");
        expect(ele._filters[0]).toEqual(
          "state:BOT_DIED",
          "pool-tag:Skia should be removed"
        );

        const chipContainer = $$(".chip_container", ele);
        expect(chipContainer).toBeTruthy(
          "there should be a filter chip container"
        );
        expect(chipContainer.children).toHaveSize(1);
        done();
      });
    });

    it("shows and hides the column selector", function (done) {
      loggedInTasklist((ele) => {
        ele._showColSelector = false;
        ele.render();

        let cs = $$(".col_selector", ele);
        expect(cs).toBeFalsy();

        let toClick = $$(".col_options", ele);
        expect(toClick).toBeTruthy("Thing to click to show col selector");
        toClick.click();

        cs = $$(".col_selector", ele);
        expect(cs).toBeTruthy();

        // click anywhere else to hide the column selector
        toClick = $$(".header", ele);
        expect(toClick).toBeTruthy("Thing to click to hide col selector");
        toClick.click();

        cs = $$(".col_selector", ele);
        expect(cs).toBeFalsy();

        done();
      });
    });

    it("filters the data it has when waiting for another request", function (done) {
      loggedInTasklist((ele) => {
        ele._cols = ["name"];
        ele._filters = [];
        ele.render();

        expect(ele._tasks).toHaveSize(22, "All 22 at the start");
        fetchMock.reset();
        mockOutRpcsWithFetchMock(true);

        const matcher = () => {
          // At this intermediate stage, before the fetch request has been sent
          // the UI will have filtered the BOT_DIED which are already in the
          // list of items. There are 2 examples in the sample data so we expect
          // a list size of 2
          expect(ele._tasks).toHaveSize(2, "2 BOT_DIED there now.");
          return true;
        };
        mockPrpc(
          fetchMock,
          "swarming.v2.Tasks",
          "ListTasks",
          [],
          matcher,
          true
        );
        ele._addFilter("state:BOT_DIED");
        fetchMock.flush(true).then(() => {
          // Since pRPC calls `.json()` on the fetch promise,
          // flush may return before rendering has been completed.
          // Therefore we use eventually, which waits until rendering is finished
          eventually(ele, (ele) => {
            expect(ele._tasks).toHaveSize(0, "none were actually returned");
            done();
          });
        });
      });
    });

    it("allows filters to be added from the search box", function (done) {
      loggedInTasklist((ele) => {
        ele._filters = [];
        ele.render();

        const filterInput = $$("#filter_search", ele);
        filterInput.value = "invalid filter";
        ele._filterSearch({ key: "Enter" });
        expect(ele._filters).toEqual([]);
        // Leave the input to let the user correct their mistake.
        expect(filterInput.value).toEqual("invalid filter");
        fetchMock.reset();
        mockOutRpcsWithFetchMock(true);
        //
        // Spy on the list call to make sure a request is made with the right filter.
        mockPrpc(
          fetchMock,
          "swarming.v2.Tasks",
          "ListTasks",
          [],
          (req) => {
            expect(req.tags).toContain("valid:filter:gpu:can:have:many:colons");
            return true;
          },
          true
        );

        filterInput.value = "valid-tag:filter:gpu:can:have:many:colons";
        ele._filterSearch({ key: "Enter" });
        expect(ele._filters).toEqual([
          "valid-tag:filter:gpu:can:have:many:colons",
        ]);
        // Empty the input for next time.
        expect(filterInput.value).toEqual("");
        filterInput.value = "valid-tag:filter:gpu:can:have:many:colons";
        ele._filterSearch({ key: "Enter" });
        // No duplicates
        expect(ele._filters).toEqual([
          "valid-tag:filter:gpu:can:have:many:colons",
        ]);

        fetchMock.flush(true).then(() => {
          done();
        });
      });
    });

    it("allows auto-corrects filters added from the search box", function (done) {
      loggedInTasklist((ele) => {
        ele._filters = [];
        ele._limit = 0; // turn off requests
        ele.render();

        const filterInput = $$("#filter_search", ele);
        filterInput.value = "state:BOT_DIED";
        ele._filterSearch({ key: "Enter" });
        expect(ele._filters).toEqual(["state:BOT_DIED"]);

        filterInput.value = "gpu-tag:something";
        ele._filterSearch({ key: "Enter" });
        expect(ele._filters).toContain("gpu-tag:something");

        // there are no valid filters that aren't a tag or state, so
        // correct those that don't have a -tag.
        filterInput.value = "gpu:something-else";
        ele._filterSearch({ key: "Enter" });
        expect(ele._filters).toContain("gpu-tag:something-else");

        done();
      });
    });

    it("searches filters by typing in the text box", function (done) {
      loggedInTasklist((ele) => {
        ele._filters = [];
        ele.render();

        const filterInput = $$("#filter_search", ele);
        filterInput.value = "dev";
        ele._refilterPrimaryKeys();

        // Auto selects the first one
        expect(ele._primaryKey).toEqual("android_devices-tag");

        let row = getChildItemWithText($$(".selector.keys"), "cpu (tag)", ele);
        expect(row).toBeFalsy("cpu (tag) should be hiding");
        row = getChildItemWithText(
          $$(".selector.keys"),
          "device_type (tag)",
          ele
        );
        expect(row).toBeTruthy("device_type (tag) should be there");
        row = getChildItemWithText($$(".selector.keys"), "stepname (tag)", ele);
        expect(row).toBeTruthy(
          "stepname (tag) should be there, because some values match"
        );

        filterInput.value = "pool:Chro";
        ele._refilterPrimaryKeys();
        // Auto selects the first one
        expect(ele._primaryKey).toEqual("pool-tag");

        row = getChildItemWithText($$(".selector.keys"), "stepname (tag)", ele);
        expect(row).toBeFalsy("stepname (tag) should be hidden");
        row = getChildItemWithText($$(".selector.keys"), "pool (tag)", ele);
        expect(row).toBeTruthy("pool (tag) should be visible");
        row = getChildItemWithText(
          $$(".selector.keys"),
          "sk_dim_pool (tag)",
          ele
        );
        expect(row).toBeFalsy("sk_dim_pool (tag) is not an exact match");

        row = getChildItemWithText($$(".selector.values"), "Chrome-perf", ele);
        expect(row).toBeTruthy("Chrome-perf should be visible");
        row = getChildItemWithText(
          $$(".selector.values"),
          "AndroidBuilder",
          ele
        );
        expect(row).toBeFalsy(
          "AndroidBuilder should be hidden, does not match Chro"
        );

        done();
      });
    });

    it("filters keys/values by partial filters", function (done) {
      loggedInTasklist((ele) => {
        ele._filters = [];
        ele.render();

        const filterInput = $$("#filter_search", ele);
        filterInput.value = "pool-tag:Ski";
        ele._refilterPrimaryKeys();

        // Auto selects the first one
        expect(ele._primaryKey).toEqual("pool-tag");

        const children = $$(".selector.keys", ele).children;
        expect(children).toHaveSize(1, "only pool-tag should show up");
        expect(children[0].textContent).toContain("pool (tag)");

        let row = getChildItemWithText($$(".selector.values"), "Chrome", ele);
        expect(row).toBeFalsy("Chrome does not match");
        row = getChildItemWithText($$(".selector.values"), "SkiaTriggers", ele);
        expect(row).toBeTruthy("SkiaTriggers matches");

        done();
      });
    });

    it("searches columns by typing in the text box", function (done) {
      loggedInTasklist((ele) => {
        ele._cols = ["name"];
        ele._showColSelector = true;
        ele.render();

        const columnInput = $$("#column_search", ele);
        columnInput.value = "build";
        ele._refilterPossibleColumns();

        const colSelector = $$(".col_selector", ele);
        expect(colSelector).toBeTruthy();
        expect(colSelector.children).toHaveSize(12); // 11 hits + the input box

        let row = getChildItemWithText(colSelector, "state");
        expect(row).toBeFalsy("state should be hiding");
        row = getChildItemWithText(colSelector, "build_is_experimental (tag)");
        expect(row).toBeTruthy("build_is_experimental (tag) should be there");

        done();
      });
    });

    it("shows and hide the extra state counts", function (done) {
      loggedInTasklist((ele) => {
        ele._allStates = false;
        ele.render();

        let countRows = $("#query_counts tr", ele);
        expect(countRows).toBeTruthy();
        expect(countRows).toHaveSize(
          1 + 7,
          "(num counts, displayed + 7 states)"
        );

        let showMore = $$(".summary expand-more-icon-sk");
        let showMore2 = $$(".summary more-horiz-icon-sk");
        let showLess = $$(".summary expand-less-icon-sk");
        expect(showMore).toBeTruthy();
        expect(showMore2).not.toHaveAttribute("hidden");
        expect(showLess).toBeFalsy();
        showMore.click();

        expect(ele._allStates).toBeTruthy();
        countRows = $("#query_counts tr", ele);
        expect(countRows).toBeTruthy();
        expect(countRows).toHaveSize(
          1 + 13,
          "(num counts, displayed + 12 states)"
        );

        showMore = $$(".summary expand-more-icon-sk");
        showMore2 = $$(".summary more-horiz-icon-sk");
        showLess = $$(".summary expand-less-icon-sk");
        expect(showMore).toBeFalsy();
        expect(showMore2).toHaveAttribute("hidden");
        expect(showLess).toBeTruthy();
        showLess.click();

        expect(ele._allStates).toBeFalsy();
        countRows = $("#query_counts tr", ele);
        expect(countRows).toBeTruthy();
        expect(countRows).toHaveSize(
          1 + 7,
          "(num counts, displayed + 7 states)"
        );

        showMore = $$(".summary expand-more-icon-sk");
        showMore2 = $$(".summary more-horiz-icon-sk");
        showLess = $$(".summary expand-less-icon-sk");
        expect(showMore).toBeTruthy();
        expect(showMore2).not.toHaveAttribute("hidden");
        expect(showLess).toBeFalsy();

        done();
      });
    });

    it("updates the links with filters and other settings", function (done) {
      loggedInTasklist((ele) => {
        ele._startTime = Timestamp.fromMilliseconds(1545151583000);
        ele._endTime = Timestamp.fromMilliseconds(1545237981000);
        ele._sort = "completedTs";
        ele._filters = ["pool-tag:Chrome", "state:DEDUPED"];
        ele.render();

        const countRows = $("#query_counts tr", ele);
        expect(countRows).toBeTruthy();

        expect(countRows[3].innerHTML).toContain("<a ", "contains a link");
        const link = $$("a", countRows[3]);
        expect(link.href).toContain(
          "at=false&c=name&c=createdTs" +
            "&c=pendingTime&c=duration&c=bot&c=pool-tag" +
            "&c=state&d=desc&et=1545237981000" +
            "&f=pool-tag%3AChrome" +
            "&f=state%3ACOMPLETED_FAILURE" +
            "&k=&n=true&s=completedTs&st=1545151583000&v=false"
        );
        done();
      });
    });

    it("updates the matching bots link", function (done) {
      loggedInTasklist((ele) => {
        ele._filters = ["device_type-tag:nemo", "state:DEDUPED"];
        ele._knownDimensions = ["device_type", "os"];
        ele.render();

        const link = $$(".options > a", ele);
        expect(link).toBeTruthy();

        expect(link.href).toContain(
          "/botlist?c=id&c=os&c=task&c=status&c=device_type" +
            "&f=device_type%3Anemo"
        );

        done();
      });
    });

    it("only tries to cancel all tasks based on tags", function (done) {
      loggedInTasklist((ele) => {
        ele.permissions.cancelTask = true;
        ele._filters = ["pool-tag:Skia", "state:PENDING", "os-tag:Windows"];
        ele.render();

        const cancelAll = $$("button#cancel_all", ele);
        expect(cancelAll).toBeTruthy();

        cancelAll.click();

        const prompt = $$("task-mass-cancel", ele);
        expect(prompt).toBeTruthy();

        expect(prompt.tags).toEqual(["pool:Skia", "os:Windows"]);

        done();
      });
    });
  }); // end describe('dynamic behavior')

  describe("api calls", function () {
    it("makes no API calls when not logged in", function (done) {
      createElement((ele) => {
        fetchMock.flush(true).then(() => {
          const calls = fetchMock.calls(MATCHED, "POST");
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
      loggedInTasklist((ele) => {
        const calls = fetchMock.calls(MATCHED, "POST");
        expect(calls).toHaveSize(
          1 + 1 + 13 + 2 + 1,
          "1 to GetTasks, 1 to GetBotDimensions, 13 to TaskCounts, 2 from swarming-app (GetPermissions, GetDetails), 1 GetPermissions (for tasks)"
        );

        checkAuthorization(calls);
        done();
      });
    });

    it("counts correctly with filters", function (done) {
      loggedInTasklist((ele) => {
        ele._filters = ["os-tag:Android"];
        fetchMock.resetHistory();
        ele._addFilter("state:PENDING_RUNNING");
        fetchMock.flush(true).then(() => {
          const calls = fetchMock.calls(MATCHED, "POST");
          expect(calls).toHaveSize(
            1 + 2 + 13,
            "1 GetPermissions, 1 to GetBotDimensions, 1 to ListTasks, 13 CountTasks"
          );

          const prpc = calls
            .filter((c) => !c[0].includes("swarming.v2.Bots"))
            .map((c) => c[1]);
          // Test that tag is always included in counts and list
          for (const rpc of prpc) {
            expect(rpc.body).toContain("os:Android");
          }
          done();
        });
      });
    });

    it("counts correctly with just states", function (done) {
      loggedInTasklist((ele) => {
        ele._filters = [];
        fetchMock.resetHistory();
        ele._addFilter("state:PENDING_RUNNING");
        fetchMock.flush(true).then(() => {
          const calls = fetchMock.calls(MATCHED, "POST");
          expect(calls).toHaveSize(
            1 + 1 + 1 + 13,
            "1 for GetBotDimensions, 1 for TaskList, 1 for GetPermissions, 13 for TaskCount"
          );

          const prpc = calls
            .filter(
              (c) =>
                !c[0].includes("GetBotDimensions") &&
                !c[0].includes("GetPermissions")
            )
            .map((c) => c[1]);
          for (const rpc of prpc) {
            expect(rpc.body).toContain("state");
          }
          done();
        });
      });
    });

    it("counts correctly when preparing to cancel", function (done) {
      loggedInTasklist((ele) => {
        ele._filters = ["pool-tag:Chrome"];
        ele.permissions.cancelTask = true;
        ele.render();
        fetchMock.resetHistory();
        const showBtn = $$("#cancel_all");
        expect(showBtn).toBeTruthy("button should exist");
        showBtn.click();

        fetchMock.flush(true).then(() => {
          expectNoUnmatchedCalls(fetchMock);
          const calls = fetchMock.calls(MATCHED, "POST");
          expect(calls).toHaveSize(2, "2 counts, 1 running, 1 pending");

          const checkForRequest = (expected) => {
            const callsCount = calls.filter((c) => {
              const rpc = c[1];
              const request = JSON.parse(rpc.body);
              return deepEquals(request, expected);
            });
            expect(callsCount).toHaveSize(
              1,
              `Expected one call to have the request ${JSON.stringify(
                expected
              )}`
            );
          };
          // This is the default new Date() value specified in the test
          // start and end are expected to be a week apart.
          const start = Timestamp.fromMilliseconds(
            Date.UTC(2018, 11, 12, 16, 46, 22, 1234)
          );
          const end = Timestamp.fromMilliseconds(
            Date.UTC(2018, 11, 19, 16, 46, 22, 1234)
          );
          checkForRequest({
            tags: ["pool:Chrome"],
            state: "QUERY_PENDING",
            start: start.toJSON(),
            end: end.toJSON(),
          });
          checkForRequest({
            tags: ["pool:Chrome"],
            state: "QUERY_RUNNING",
            start: start.date.toJSON(),
            end: end.toJSON(),
          });
          done();
        });
      });
    });

    it("counts correctly when cancelling", function (done) {
      jasmine.clock().uninstall(); // re-enable setTimeout
      mockPrpc(fetchMock, "swarming.v2.Tasks", "CancelTasks", { matched: 10 });
      loggedInTasklist((ele) => {
        ele._filters = ["pool-tag:Chrome"];
        ele.permissions.cancelTask = true;
        ele.render();
        const showBtn = $$("#cancel_all");
        expect(showBtn).toBeTruthy("show button should exist");
        showBtn.click();

        fetchMock.flush(true).then(() => {
          // The Promise.all for when the fetches complete isn't always
          // done, so wait until next microtask or so to make sure that promise
          // gets a chance to execute.
          setTimeout(() => {
            fetchMock.resetHistory();
            const cancelBtn = $$("task-mass-cancel button.cancel");
            expect(cancelBtn).toBeTruthy("cancel button should exist");
            expect(cancelBtn).not.toHaveAttribute("disabled");
            cancelBtn.click();

            fetchMock.flush(true).then(() => {
              expectNoUnmatchedCalls(fetchMock);
              let calls = fetchMock.calls(MATCHED, "GET");
              expect(calls).toHaveSize(0, "Only posts");
              calls = fetchMock.calls(MATCHED, "POST");
              expect(calls).toHaveSize(1, "1 cancel request");
              // calls is an array of 2-length arrays with the first element
              // being the string of the url and the second element being
              // the options that were passed in
              const cancelPost = calls[0];
              expect(cancelPost[0]).toContain(
                "/prpc/swarming.v2.Tasks/CancelTasks"
              );

              const req = JSON.parse(cancelPost[1].body);
              delete req["start"];
              delete req["end"];
              expect(req).toEqual({ limit: 100, tags: ["pool:Chrome"] });

              done();
            });
          }, 50);
        });
      });
    });
  }); // end describe('api calls')

  describe("data parsing", function () {
    const ANDROID_TASK = tasks22.items[0];

    it("turns the dates into DateObjects", function () {
      // Make a copy of the object because processTasks will modify it in place.
      const tasks = processTasks([deepCopy(ANDROID_TASK)], {});
      const task = tasks[0];
      expect(task.createdTs).toBeTruthy();
      expect(task.createdTs instanceof Date).toBeTruthy(
        "Should be a date object"
      );
      expect(task.humanized.time.createdTs).toBeTruthy();
      expect(task.pendingTime).toBeTruthy();
      expect(task.humanPendingTime).toBeTruthy();
    });

    it("gracefully handles null data", function () {
      const tasks = processTasks(null);

      expect(tasks).toBeTruthy();
      expect(tasks).toHaveSize(0);
    });

    it("produces a list of tags", function () {
      const tags = {};
      const tasks = processTasks(deepCopy(tasks22.items), tags);
      const keys = Object.keys(tags);
      expect(keys).toBeTruthy();
      expect(keys).toHaveSize(75);
      expect(keys).toContain("pool");
      expect(keys).toContain("purpose");
      expect(keys).toContain("source_revision");

      expect(tasks).toHaveSize(22);
    });

    it("filters tasks based on special keys", function () {
      const tasks = processTasks(deepCopy(tasks22.items), {});

      expect(tasks).toBeTruthy();
      expect(tasks).toHaveSize(22);

      const filtered = filterTasks(["state:COMPLETED_FAILURE"], tasks);
      expect(filtered).toHaveSize(2);
      const expectedIds = ["41e0310fe0b7c410", "41e031b2c8b46710"];
      const actualIds = filtered.map((task) => task.taskId);
      actualIds.sort();
      expect(actualIds).toEqual(expectedIds);
    });

    it("filters tasks based on tags", function () {
      const tasks = processTasks(deepCopy(tasks22.items), {});

      expect(tasks).toBeTruthy();
      expect(tasks).toHaveSize(22);

      let filtered = filterTasks(["pool-tag:Chrome"], tasks);
      expect(filtered).toHaveSize(7);
      let actualIds = filtered.map((task) => task.taskId);
      expect(actualIds).toContain("41e0204f39d06210"); // spot check
      expect(actualIds).not.toContain("41e0182a00fcc110");

      // some tasks have multiple 'purpose' tags
      filtered = filterTasks(["purpose-tag:luci"], tasks);
      expect(filtered).toHaveSize(10);
      actualIds = filtered.map((task) => task.taskId);
      expect(actualIds).toContain("41e020504d0a5110"); // spot check
      expect(actualIds).not.toContain("41e0310fe0b7c410");

      filtered = filterTasks(["pool-tag:Skia", "gpu-tag:none"], tasks);
      expect(filtered).toHaveSize(1);
      expect(filtered[0].taskId).toBe("41e031b2c8b46710");

      filtered = filterTasks(
        ["pool-tag:Skia", "gpu-tag:10de:1cb3-384.59"],
        tasks
      );
      expect(filtered).toHaveSize(2);
      actualIds = filtered.map((task) => task.taskId);
      expect(actualIds).toContain("41dfa79d3bf29010");
      expect(actualIds).toContain("41df677202f20310");

      filtered = filterTasks(["state:DEDUPED"], tasks);
      expect(filtered).toHaveSize(2);
      actualIds = filtered.map((task) => task.taskId);
      expect(actualIds).toContain("41e0284bc3ef4f10");
      expect(actualIds).toContain("41e0284bf01aef10");
    });

    it("correctly converts from a legacy state to a new state", function () {
      const oldState = {
        s: "created_ts",
        c: ["modified_ts"],
      };
      convertFromLegacyState(oldState);
      expect(oldState.s).toEqual("createdTs");
      expect(oldState.c).toEqual(["modifiedTs"]);
    });

    it("correctly makes query params from filters", function () {
      // We know query.fromObject is used and it puts the query params in
      // a deterministic, sorted order. This means we can compare
      const expectations = [
        {
          // basic 'state'
          extra: {
            limit: 7,
            start: 12345678,
            end: 456789012,
          },
          filters: ["state:BOT_DIED"],
          output: {
            limit: 7,
            start: 12345678,
            end: 456789012,
            state: "QUERY_BOT_DIED",
          },
        },
        {
          // two tags
          extra: {
            limit: 342,
            start: 12345678,
            end: 456789012,
          },
          filters: ["os-tag:Window", "gpu-tag:10de"],
          output: {
            limit: 342,
            start: 12345678,
            end: 456789012,
            tags: ["os:Window", "gpu:10de"],
          },
        },
        {
          // tags and state
          extra: {
            limit: 57,
            start: 12345678,
            end: 456789012,
          },
          filters: ["os-tag:Window", "state:RUNNING", "gpu-tag:10de"],
          output: {
            start: 12345678,
            end: 456789012,
            limit: 57,
            state: "QUERY_RUNNING",
            tags: ["os:Window", "gpu:10de"],
          },
        },
      ];

      for (const testcase of expectations) {
        const qp = listQueryParams(testcase.filters, testcase.extra);
        expect(qp).toEqual(testcase.output);
      }
      const testcase = expectations[0];
      testcase.extra.cursor = "mock_cursor12345";
      const qp = listQueryParams(testcase.filters, testcase.extra);
      const expected = {
        ...expectations[0].output,
        cursor: "mock_cursor12345",
      };
      expect(qp).toEqual(expected);
    });
  }); // end describe('data parsing')
});
