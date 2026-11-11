// Copyright 2018 The LUCI Authors.
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

import "./oauth-login";
import fetchMock from "fetch-mock";

describe("oauth-login", function () {
  // A reusable HTML element in which we create our element under test.
  const container = document.createElement("div");
  document.body.appendChild(container);

  afterEach(function () {
    container.innerHTML = "";
  });

  // ===============TESTS START====================================

  describe("testing-offline true", function () {
    // calls the test callback with one element 'ele', a created <oauth-login>.
    function createElement(test) {
      return window.customElements.whenDefined("oauth-login").then(() => {
        container.innerHTML = `<oauth-login testing_offline=true></oauth-login>`;
        expect(container.firstElementChild).toBeTruthy();
        expect(container.firstElementChild.testing_offline).toBeTruthy();
        test(container.firstElementChild);
      });
    }

    it("starts off logged out", function (done) {
      createElement((ele) => {
        expect(ele.authHeader).toBe("");
        done();
      });
    });

    it("triggers a log-in custom event on login", function (done) {
      createElement((ele) => {
        ele.addEventListener("log-in", (e) => {
          e.stopPropagation();
          expect(e.detail).toBeDefined();
          expect(e.detail.authHeader).toContain("Bearer ");
          done();
        });
        ele._logIn();
      });
    });

    it("has authHeader set after log-in", function (done) {
      createElement((ele) => {
        ele._logIn();
        expect(ele.authHeader).toContain("Bearer ");
        done();
      });
    });
  }); // end describe('testing-offline true')

  describe("testing-offline false", function () {
    // calls the test callback with one element 'ele', a created <oauth-login>.
    function createElement(test) {
      return window.customElements.whenDefined("oauth-login").then(() => {
        container.innerHTML = `<oauth-login></oauth-login>`;
        expect(container.firstElementChild).toBeTruthy();
        expect(container.firstElementChild.testing_offline).toBeFalsy();
        test(container.firstElementChild);
      });
    }

    function mockState(accessToken, extra = {}) {
      fetchMock.get(
        "/auth/openid/state",
        {
          identity: "user:someone@example.com",
          email: "someone@example.com",
          picture: "http://example.com/picture.jpg",
          accessToken: accessToken,
          ...extra,
        },
        { overwriteRoutes: true }
      );
    }

    beforeEach(function () {
      jasmine.clock().install();
    });

    afterEach(function () {
      jasmine.clock().uninstall();
      // Completely remove the mocking which allows each test
      // to be able to mess with the mocked routes w/o impacting other tests.
      fetchMock.reset();
    });

    it("fetches state and fires log-in event", function (done) {
      mockState("12345-zzzzzz");
      createElement((ele) => {
        ele.addEventListener("log-in", (e) => {
          expect(ele.authHeader).toEqual("Bearer 12345-zzzzzz");
          expect(ele.profile).toEqual({
            email: "someone@example.com",
            imageURL: "http://example.com/picture.jpg",
          });
          expect(e.detail.refresh).toBeFalse();
          done();
        });
      });
    });

    it("refreshes 120s before expiry and clamps TTLs", function (done) {
      jasmine.clock().mockDate(new Date(1000 * 1000));
      mockState("token-1", { accessTokenExpiresIn: 180 });
      createElement((ele) => {
        let step = 0;
        ele.addEventListener("log-in", (e) => {
          step++;
          if (step === 1) {
            expect(ele.authHeader).toEqual("Bearer token-1");
            expect(e.detail.refresh).toBeFalse();
            mockState("token-2", { accessTokenExpiry: 1000 });
            jasmine.clock().tick(59 * 1000);
            expect(fetchMock.calls("/auth/openid/state")).toHaveSize(1);
            jasmine.clock().tick(1000);
          } else if (step === 2) {
            expect(ele.authHeader).toEqual("Bearer token-2");
            expect(e.detail.refresh).toBeTrue();
            mockState("token-3", { accessTokenExpiresIn: 3600 });
            jasmine.clock().tick(10 * 1000);
          } else if (step === 3) {
            expect(ele.authHeader).toEqual("Bearer token-3");
            expect(e.detail.refresh).toBeTrue();
            done();
          }
        });
      });
    });

    it("retries on transient error and clears state when session expires", function (done) {
      mockState("token-1", { accessTokenExpiresIn: 20 });
      createElement((ele) => {
        let step = 0;
        ele.addEventListener("log-in", (e) => {
          step++;
          if (step === 1) {
            fetchMock.get("/auth/openid/state", 500, { overwriteRoutes: true });
            jasmine.clock().tick(10 * 1000);
            fetchMock.flush(true).then(() => {
              expect(ele.authHeader).toEqual("Bearer token-1");
              mockState("token-2", { accessTokenExpiresIn: 180 });
              jasmine.clock().tick(15 * 1000);
            });
          } else if (step === 2) {
            expect(ele.authHeader).toEqual("Bearer token-2");
            expect(e.detail.refresh).toBeTrue();
            fetchMock.get(
              "/auth/openid/state",
              { identity: "anonymous:anonymous" },
              { overwriteRoutes: true }
            );
            jasmine.clock().tick(60 * 1000);
          } else if (step === 3) {
            expect(e.detail.authHeader).toEqual("");
            expect(e.detail.profile).toBeNull();
            expect(e.detail.refresh).toBeFalse();
            expect(ele.authHeader).toEqual("");
            container.innerHTML = "";
            jasmine.clock().tick(60 * 1000);
            expect(fetchMock.calls("/auth/openid/state")).toHaveSize(4);
            done();
          }
        });
      });
    });
  }); // end describe('testing-offline false')
});
