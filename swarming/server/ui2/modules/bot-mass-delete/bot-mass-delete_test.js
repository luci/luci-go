// Copyright 2019 The LUCI Authors.
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

import "./bot-mass-delete";
import fetchMock from "fetch-mock";
import { mockPrpc, deepEquals } from "../test_util";
import { $, $$ } from "common-sk/modules/dom";
import {
  customMatchers,
  expectNoUnmatchedCalls,
  mockUnauthorizedSwarmingService,
  MATCHED,
} from "../test_util";

describe("bot-mass-delete", function () {
  // A reusable HTML element in which we create our element under test.
  const container = document.createElement("div");
  document.body.appendChild(container);

  beforeEach(function () {
    jasmine.addMatchers(customMatchers);

    mockUnauthorizedSwarmingService(fetchMock, { delete_bot: true });

    // Everything else
    fetchMock.catch(404);
  });

  afterEach(function () {
    container.innerHTML = "";
    // Completely remove the mocking which allows each test
    // to be able to mess with the mocked routes w/o impacting other tests.
    fetchMock.reset();
  });

  // ===============TESTS START====================================

  // calls the test callback with one element 'ele', a created <bot-mass-delete>.
  function createElement(test) {
    return window.customElements.whenDefined("bot-mass-delete").then(() => {
      container.innerHTML = `<bot-mass-delete dimensions="pool:Chrome,os:Android" authHeader="fake"></bot-mass-delete>`;
      expect(container.firstElementChild).toBeTruthy();
      test(container.firstElementChild);
    });
  }

  it("can read in attributes", function (done) {
    createElement((ele) => {
      expect(ele.dimensions).toHaveSize(2);
      expect(ele.dimensions).toContain({ key: "pool", value: "Chrome" });
      expect(ele.dimensions).toContain({ key: "os", value: "Android" });
      expect(ele.authHeader).toBe("fake");
      done();
    });
  });

  it("has a list of the passed in dimensions", function (done) {
    createElement((ele) => {
      const listedDims = $("ul li", ele);
      expect(listedDims).toHaveSize(2);
      expect(listedDims[0]).toMatchTextContent("os:Android");
      done();
    });
  });

  it("makes no API calls if show() is not calleds", function (done) {
    createElement((ele) => {
      expectNoUnmatchedCalls(fetchMock);
      done();
    });
  });

  it("makes an API call to count when loading", function (done) {
    createElement((ele) => {
      mockPrpc(
        fetchMock,
        "swarming.v2.Bots",
        "CountBots",
        { dead: 532 },
        (req) =>
          deepEquals(req.dimensions, [
            { key: "os", value: "Android" },
            { key: "pool", value: "Chrome" },
          ])
      );

      ele.show();
      // The true on flush waits for res.json() to resolve too, which
      // is when we know the element has updated the _tasks.
      fetchMock.flush(true).then(() => {
        expectNoUnmatchedCalls(fetchMock);
        const calls = fetchMock.calls(MATCHED, "POST");
        expect(calls).toHaveSize(1);
        done();
      });
    });
  });

  it("makes an API call to list after clicking, then deletes", function (done) {
    createElement((ele) => {
      // create a shortened version of the returned data
      mockPrpc(
        fetchMock,
        "swarming.v2.Bots",
        "ListBots",
        {
          items: [{ botId: "bot-1" }, { botId: "bot-2" }, { botId: "bot-3" }],
        },
        (req) => {
          return (
            deepEquals(req.dimensions, [
              { key: "os", value: "Android" },
              { key: "pool", value: "Chrome" },
            ]) &&
            req.limit === 200 &&
            req.isDead === "TRUE"
          );
        }
      );

      mockPrpc(fetchMock, "swarming.v2.Bots", "DeleteBot", {}, (req) => {
        return ["bot-1", "bot-2", "bot-3"].includes(req.botId);
      });

      let sawStartEvent = false;
      ele.addEventListener("bots-deleting-started", () => {
        sawStartEvent = true;
      });

      ele.addEventListener("bots-deleting-finished", () => {
        expect(sawStartEvent).toBeTruthy();
        expectNoUnmatchedCalls(fetchMock);
        const calls = fetchMock.calls(MATCHED, "POST");
        expect(calls).toHaveSize(4, "1 to get, 3 to delete");
        done();
      });

      ele._readyToDelete = true;
      ele.render();
      const button = $$("button.delete", ele);
      button.click();
    });
  });

  it("pages the bot list calls before deleting", function (done) {
    createElement((ele) => {
      // create a shortened version of the returned data
      let first = true;
      mockPrpc(
        fetchMock,
        "swarming.v2.Bots",
        "ListBots",
        (_req) => {
          if (first) {
            first = false;
            ele.authHeader = "Bearer refreshed-token";
            return {
              items: [{ botId: "bot-1" }],
              cursor: "alpha",
            };
          } else {
            return {
              items: [{ botId: "bot-2" }, { botId: "bot-3" }],
            };
          }
        },
        (req) => {
          return (
            deepEquals(req.dimensions, [
              { key: "os", value: "Android" },
              { key: "pool", value: "Chrome" },
            ]) &&
            req.limit === 200 &&
            req.isDead === "TRUE"
          );
        }
      );

      mockPrpc(fetchMock, "swarming.v2.Bots", "DeleteBot", {}, (req) => {
        return ["bot-1", "bot-2", "bot-3"].includes(req.botId);
      });

      let sawStartEvent = false;
      ele.addEventListener("bots-deleting-started", () => {
        sawStartEvent = true;
      });

      ele.addEventListener("bots-deleting-finished", () => {
        expect(sawStartEvent).toBeTruthy();
        expectNoUnmatchedCalls(fetchMock);
        const calls = fetchMock.calls(MATCHED, "POST");
        expect(calls).toHaveSize(3 + 2, "3 to delete, 2 from list");
        expect(calls[0][1].headers.authorization).toBe("fake");
        calls.slice(1).forEach((c) => {
          expect(c[1].headers.authorization).toBe("Bearer refreshed-token");
        });
        done();
      });

      ele._readyToDelete = true;
      ele.render();
      const button = $$("button.delete", ele);
      button.click();
    });
  });
});
