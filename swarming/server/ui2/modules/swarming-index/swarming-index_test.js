// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import "./swarming-index";
import fetchMock from "fetch-mock";
import {
  expectNoUnmatchedCalls,
  mockUnauthorizedSwarmingService,
  MATCHED,
  mockUnauthorizedPrpc,
  mockPrpc,
} from "../test_util";

describe("swarming-index", function () {
  beforeEach(function () {
    // These are the default responses to the expected API calls (aka 'matched')
    // They can be overridden for specific tests, if needed.
    mockUnauthorizedSwarmingService(fetchMock, {
      getBootstrapToken: false,
    });

    mockUnauthorizedPrpc(fetchMock, "swarming.v2.Swarming", "GetToken");

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

  // calls the test callback with one element 'ele', a created <swarming-index>.
  // We can't put the describes inside the whenDefined callback because
  // that doesn't work on Firefox (and possibly other places).
  function createElement(test) {
    return window.customElements.whenDefined("swarming-index").then(() => {
      container.innerHTML = `<swarming-index testing_offline=true>
          </swarming-index>`;
      expect(container.firstElementChild).toBeTruthy();
      test(container.firstElementChild);
    });
  }

  function userLogsIn(ele, callback) {
    // The swarming-app emits the 'busy-end' event when all pending
    // fetches (and renders) have resolved.
    let ran = false;
    ele.addEventListener("busy-end", (_e) => {
      if (!ran) {
        callback();
      }
      ran = true; // prevent multiple runs if the test makes the
      // app go busy (e.g. if it calls fetch).
    });
    const login = ele.querySelector("oauth-login");
    login._logIn();
    fetchMock.flush();
  }

  function becomeAdmin() {
    // overwrite the default fetchMock behaviors for this run to return
    // what an admin would see.
    mockPrpc(
      fetchMock,
      "swarming.v2.Swarming",
      "GetPermissions",
      { getBootstrapToken: true },
      undefined,
      true
    );
    mockPrpc(
      fetchMock,
      "swarming.v2.Swarming",
      "GetToken",
      {
        bootstrapToken: "8675309JennyDontChangeYourNumber8675309",
      },
      undefined,
      true
    );
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
          const serverVersion = ele.querySelector(
            "swarming-app>main .server_version"
          );
          expect(serverVersion).toBeTruthy();
          expect(serverVersion.innerText).toContain("must log in");
          done();
        });
      });
      it("does not display the bootstrapping section", function (done) {
        createElement((ele) => {
          const sectionHeaders = ele.querySelectorAll("swarming-app>main h2");
          expect(sectionHeaders).toBeTruthy();
          expect(sectionHeaders).toHaveSize(2);
          done();
        });
      });
    });

    describe("when logged in as unauthorized user", function () {
      function notAuthorized() {
        // overwrite the default fetchMock behaviors to have everything return 403.
        mockUnauthorizedPrpc(fetchMock, "swarming.v2.Swarming", "GetDetails");
        mockPrpc(
          fetchMock,
          "swarming.v2.Swarming",
          "GetPermissions",
          {},
          undefined,
          true
        );
      }

      beforeEach(notAuthorized);

      it("tells the user to try a different account", function (done) {
        createElement((ele) => {
          userLogsIn(ele, () => {
            const serverVersion = ele.querySelector(
              "swarming-app>main .server_version"
            );
            expect(serverVersion).toBeTruthy();
            expect(serverVersion.innerText).toContain("different account");
            done();
          });
        });
      });
      it("does not displays the bootstrapping section", function (done) {
        createElement((ele) => {
          userLogsIn(ele, () => {
            const sectionHeaders = ele.querySelectorAll("swarming-app>main h2");
            expect(sectionHeaders).toBeTruthy();
            expect(sectionHeaders).toHaveSize(2);
            done();
          });
        });
      });
      it("does not display the bootstrap token", function (done) {
        createElement((ele) => {
          userLogsIn(ele, () => {
            const commandBox = ele.querySelector("swarming-app>main .command");
            expect(commandBox).toBeNull();
            done();
          });
        });
      });
    });

    describe("when logged in as user (no bootstrap_token)", function () {
      it("displays the server version", function (done) {
        createElement((ele) => {
          userLogsIn(ele, () => {
            const serverVersion = ele.querySelector(
              "swarming-app>main .server_version"
            );
            expect(serverVersion).toBeTruthy();
            expect(serverVersion.innerText).toContain("1234-abcdefg");
            done();
          });
        });
      });
      it("does not displays the bootstrapping section", function (done) {
        createElement((ele) => {
          userLogsIn(ele, () => {
            const sectionHeaders = ele.querySelectorAll("swarming-app>main h2");
            expect(sectionHeaders).toBeTruthy();
            expect(sectionHeaders).toHaveSize(2);
            done();
          });
        });
      });
      it("does not display the bootstrap token", function (done) {
        createElement((ele) => {
          userLogsIn(ele, () => {
            const commandBox = ele.querySelector("swarming-app>main .command");
            expect(commandBox).toBeNull();
            done();
          });
        });
      });
    });

    describe("when logged in as admin (boostrap_token)", function () {
      beforeEach(becomeAdmin);

      it("displays the server version", function (done) {
        createElement((ele) => {
          userLogsIn(ele, () => {
            const serverVersion = ele.querySelector(
              "swarming-app>main .server_version"
            );
            expect(serverVersion).toBeTruthy();
            expect(serverVersion.innerText).toContain("1234-abcdefg");
            done();
          });
        });
      });
      it("displays the bootstrapping section", function (done) {
        createElement((ele) => {
          userLogsIn(ele, () => {
            const sectionHeaders = ele.querySelectorAll("swarming-app>main h2");
            expect(sectionHeaders).toBeTruthy();
            expect(sectionHeaders).toHaveSize(3);
            done();
          });
        });
      });
      it("displays the bootstrap token", function (done) {
        createElement((ele) => {
          userLogsIn(ele, () => {
            // There are several of these, but we'll just check one of them.
            const commandBox = ele.querySelector("swarming-app>main .command");
            expect(commandBox).toBeTruthy();
            expect(commandBox.innerText).toContain("8675309");
            done();
          });
        });
      });
    });
  }); // end describe('html structure')

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

    it("does not request a token when a normal user logs in", function (done) {
      createElement((ele) => {
        userLogsIn(ele, () => {
          const calls = fetchMock.calls(MATCHED, "POST");
          expect(calls).toHaveSize(
            2,
            "2 from swarming-app (GetPermissions, GetDetails)"
          );

          expectNoUnmatchedCalls(fetchMock);
          done();
        });
      });
    });

    it("fetches a token when an admin logs in", function (done) {
      becomeAdmin();
      createElement((ele) => {
        userLogsIn(ele, () => {
          const calls = fetchMock.calls(MATCHED, "POST");
          const posts = calls.map((c) => c[0]);
          expect(calls).toHaveSize(
            3,
            "2 from swarming-app (GetPermissions, GetDetails), 1 GetToken"
          );
          // Only call is `GetToken`
          expect(
            posts.filter((c) => c.endsWith("swarming.v2.Swarming/GetToken"))
          ).toHaveSize(1);

          // check authorization headers are set
          calls.forEach((c) => {
            expect(c[1].headers).toBeDefined();
            expect(c[1].headers.authorization).toContain("Bearer ");
          });

          expectNoUnmatchedCalls(fetchMock);
          done();
        });
      });
    });
  }); // end describe('api calls')
});
