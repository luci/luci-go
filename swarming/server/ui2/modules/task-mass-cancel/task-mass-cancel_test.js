// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import "./task-mass-cancel";
import fetchMock from "fetch-mock";
import { mockPrpc } from "../test_util";
import { Timestamp } from "../task-list/task-list-helpers";
import { $, $$ } from "common-sk/modules/dom";
import {
  customMatchers,
  expectNoUnmatchedCalls,
  mockUnauthorizedSwarmingService,
  MATCHED,
} from "../test_util";

// A reusable HTML element in which we create our element under test.
const container = document.createElement("div");
document.body.appendChild(container);

describe("task-mass-cancel", function () {
  beforeEach(function () {
    jasmine.addMatchers(customMatchers);

    mockUnauthorizedSwarmingService(fetchMock, { delete_bot: true });

    mockPrpc(fetchMock, "swarming.v2.Tasks", "CountTasks", { count: 17 });

    // Everything else
    fetchMock.catch(404);
  });

  afterEach(function () {
    container.innerHTML = "";
    // Completely remove the mocking which allows each test
    // to be able to mess with the mocked routes w/o impacting other tests.
    fetchMock.reset();
  });

  // calls the test callback with one element 'ele', a created <task-mass-cancel>.
  function createElement(test) {
    return window.customElements.whenDefined("task-mass-cancel").then(() => {
      container.innerHTML = `<task-mass-cancel start=10 end=20 tags="pool:Chrome,os:Android" authHeader="fake"></task-mass-cancel>`;
      expect(container.firstElementChild).toBeTruthy();
      test(container.firstElementChild);
    });
  }

  // ===============TESTS START====================================

  it("can read in attributes", function (done) {
    createElement((ele) => {
      expect(ele.tags).toHaveSize(2);
      expect(ele.tags).toContain("pool:Chrome");
      expect(ele.tags).toContain("os:Android");
      expect(ele.authHeader).toBe("fake");
      done();
    });
  });

  it("has a list of the passed in dimensions", function (done) {
    createElement((ele) => {
      const tags = $("ul li", ele);
      expect(tags).toHaveSize(2);
      expect(tags[0]).toMatchTextContent("os:Android");
      done();
    });
  });

  it("makes 2 API calls to count when loading", function (done) {
    createElement((ele) => {
      ele.show();
      // The true on flush waits for res.json() to resolve too, which
      // is when we know the element has updated the _tasks.
      fetchMock.flush(true).then(() => {
        expectNoUnmatchedCalls(fetchMock);
        const calls = fetchMock.calls(MATCHED, "POST");
        expect(calls).toHaveSize(2);
        done();
      });
    });
  });

  it("makes an API call to delete after clicking", function (done) {
    createElement((ele) => {
      mockPrpc(fetchMock, "swarming.v2.Tasks", "CancelTasks", { matched: 22 });

      let sawStartEvent = false;
      ele.addEventListener("tasks-canceling-started", () => {
        sawStartEvent = true;
      });

      ele.addEventListener("tasks-canceling-finished", () => {
        expect(sawStartEvent).toBeTruthy();
        expectNoUnmatchedCalls(fetchMock);

        const calls = fetchMock.calls(MATCHED, "POST");
        expect(calls).toHaveSize(1, "1 to delete");
        const req = calls[0];
        const payload = JSON.parse(req[1].body);
        const expected = {
          limit: 100,
          tags: ["os:Android", "pool:Chrome"],
          start: Timestamp.fromMilliseconds(10).toJSON(),
          end: Timestamp.fromMilliseconds(20).toJSON(),
        };
        expect(payload).toEqual(expected);
        done();
      });

      ele._readyToCancel = true;
      ele.render();
      const button = $$("button.cancel", ele);
      button.click();
    });
  });
});
