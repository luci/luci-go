// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

    afterEach(function () {
      // Completely remove the mocking which allows each test
      // to be able to mess with the mocked routes w/o impacting other tests.
      fetchMock.reset();
    });

    it("fetches state and fires log-in event", function (done) {
      fetchMock.get("/auth/openid/state", {
        identity: "user:someone@example.com",
        email: "someone@example.com",
        picture: "http://example.com/picture.jpg",
        accessToken: "12345-zzzzzz",
      });
      createElement((ele) => {
        ele.addEventListener("log-in", () => {
          expect(ele.authHeader).toEqual("Bearer 12345-zzzzzz");
          expect(ele.profile).toEqual({
            email: "someone@example.com",
            imageURL: "http://example.com/picture.jpg",
          });
          done();
        });
      });
    });
  }); // end describe('testing-offline false')
});
