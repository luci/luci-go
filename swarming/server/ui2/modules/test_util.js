// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

/** @module swarming-ui/test_util
 * @description
 *
 * <p>
 *  A general set of useful functions for tests and demos,
 *  e.g. reducing boilerplate.
 * </p>
 */

export const UNMATCHED = false;

export const customMatchers = {
  // see https://jasmine.github.io/tutorials/custom_matcher
  // for docs on the factory that returns a matcher.
  toContainRegex: function (util, customEqualityTesters) {
    return {
      compare: function (actual, regex) {
        if (!(regex instanceof RegExp)) {
          throw new Error(
            `toContainRegex expects a regex, got ${JSON.stringify(regex)}`
          );
        }
        const result = {};

        if (!actual || !actual.length) {
          result.pass = false;
          result.message =
            `Expected ${actual} to be a non-empty array ` +
            `containing something matching ${regex}`;
          return result;
        }
        for (const s of actual) {
          if (s.match && s.match(regex)) {
            result.pass = true;
            // craft the message for the negated version (i.e. using .not)
            result.message =
              `Expected ${actual} not to have anyting ` +
              `matching ${regex}, but ${s} did`;
            return result;
          }
        }
        result.message = `Expected ${actual} to have element matching ${regex}`;
        result.pass = false;
        return result;
      },
    };
  },

  toHaveAttribute: function (util, customEqualityTesters) {
    return {
      compare: function (actual, attribute) {
        if (!isElement(actual)) {
          throw new Error(`${actual} is not a DOM element`);
        }
        return {
          pass: actual.hasAttribute(attribute),
        };
      },
    };
  },

  // Trims off whitespace before comparing
  toMatchTextContent: function (util, customEqualityTesters) {
    return {
      compare: function (actual, text) {
        if (!isElement(actual)) {
          throw new Error(`${actual} is not a DOM element`);
        }
        function normalize(s) {
          return s.trim().replace("\t", " ").replace(/ {2,}/g, " ");
        }
        text = normalize(text);
        const actualText = normalize(actual.innerText);
        if (actualText === text) {
          return {
            // craft the message for the negated version
            message:
              `Expected ${actualText} to not equal ${text} ` +
              `(ignoring whitespace)`,
            pass: true,
          };
        }
        return {
          message:
            `Expected ${actualText} to equal ${text} ` +
            `(ignoring whitespace)`,
          pass: false,
        };
      },
    };
  },
};

function isElement(ele) {
  // https://stackoverflow.com/a/36894871
  return ele instanceof Element || ele instanceof HTMLDocument;
}

export function mockUnauthorizedSwarmingService(
  fetchMock,
  permissions,
  serverDetails = {}
) {
  fetchMock.get("/auth/openid/state", {
    identity: "anonymous:anonymous",
  });

  const defaultServerDetails = {
    serverVersion: "1234-abcdefg",
    botVersion: "abcdoeraymeyouandme",
    machineProviderTemplate: "https://example.com/leases/%s",
    displayServerUrlTemplate: "https://example.com#id=%s",
  };
  mockPrpc(fetchMock, "swarming.v2.Swarming", "GetDetails", {
    ...defaultServerDetails,
    ...serverDetails,
  });
  mockPrpc(fetchMock, "swarming.v2.Swarming", "GetPermissions", permissions);
}

export function mockAuthorizedSwarmingService(
  fetchMock,
  permissions,
  serverDetails = {}
) {
  fetchMock.get("/auth/openid/state", {
    identity: "user:someone@example.com",
    email: "someone@example.com",
    picture: "http://example.com/picture.jpg",
    accessToken: "12345-zzzzzz",
  });

  const defaultServerDetails = {
    serverVersion: "1234-abcdefg",
    botVersion: "abcdoeraymeyouandme",
    machineProviderTemplate: "https://example.com/leases/%s",
    displayServerUrlTemplate: "https://example.com#id=%s",
  };
  mockPrpc(fetchMock, "swarming.v2.Swarming", "GetDetails", {
    ...defaultServerDetails,
    ...serverDetails,
  });
  mockPrpc(fetchMock, "swarming.v2.Swarming", "GetPermissions", permissions);
}

export function requireLogin(loggedIn, delay = 100) {
  const originalItems = loggedIn.items && loggedIn.items.slice();
  return function (url, opts) {
    if (opts && opts.headers && opts.headers.authorization) {
      return new Promise((resolve) => {
        setTimeout(resolve, delay);
      }).then(() => {
        if (loggedIn.items instanceof Array) {
          // pretend there are two pages
          if (!loggedIn.cursor) {
            // first page
            loggedIn.cursor = "fake_cursor12345";
            loggedIn.items = originalItems.slice(0, originalItems.length / 2);
          } else {
            // second page
            loggedIn.cursor = undefined;
            loggedIn.items = originalItems.slice(originalItems.length / 2);
          }
        }
        if (loggedIn instanceof Function) {
          const val = loggedIn(url, opts);
          if (!val) {
            return {
              status: 404,
              body: JSON.stringify({ error: { message: "bot not found." } }),
              headers: { "content-type": "application/json" },
            };
          }
          return {
            status: 200,
            body: JSON.stringify(val),
            headers: { "content-type": "application/json" },
          };
        }
        return {
          status: 200,
          body: JSON.stringify(loggedIn),
          headers: { "content-type": "application/json" },
        };
      });
    } else {
      return new Promise((resolve) => {
        setTimeout(resolve, delay);
      }).then(() => {
        return {
          status: 403,
          body: "Try logging in",
          headers: { "content-type": "text/plain" },
        };
      });
    }
  };
}

/** childrenAsArray looks at an HTML element and returns the children
 *  as a real array (e.g. with .forEach)
 */
export function childrenAsArray(ele) {
  return Array.prototype.slice.call(ele.children);
}

/** expectNoUnmatchedCalls assets that there were no
 *  unexpected (unmatched) calls to fetchMock.
 */
export function expectNoUnmatchedCalls(fetchMock) {
  let calls = fetchMock.calls(UNMATCHED, "GET");
  expect(calls).toHaveSize(0, "no unmatched (unexpected) GETs");
  if (calls.length) {
    console.warn("unmatched GETS", calls);
  }
  calls = fetchMock.calls(UNMATCHED, "POST");
  expect(calls).toHaveSize(0, "no unmatched (unexpected) POSTs");
  if (calls.length) {
    console.warn("unmatched POSTS", calls);
  }
}

/** getChildItemWithText looks at the children of the given element
 *  and returns the element that has textContent that matches the
 *  passed in value.
 */
export function getChildItemWithText(ele, value) {
  expect(ele).toBeTruthy();

  for (let i = 0; i < ele.children.length; i++) {
    const child = ele.children[i];
    const text = child.firstElementChild;
    if (text && text.textContent.trim() === value) {
      return child;
    }
  }
  // uncomment below when debugging
  // fail(`Could not find child of ${ele} with text value ${value}`);
  return null;
}

export const MATCHED = true;

const stringify = function (data) {
  return `)]}'${JSON.stringify(data)}`;
};

const prpcHeaders = {
  "x-prpc-grpc-code": "0",
  "content-type": "application/json",
};

export function prpcResponse(data) {
  return (_url, _opts) =>
    new Response(stringify(data), {
      status: 200,
      headers: prpcHeaders,
    });
}

/**
 * @callback fn
 * @param {string} url url sent to fetch for the request
 * @param {Object} opts options for the request object being created by fetch-mock
 *
 * @return {Response} response object - see: https://developer.mozilla.org/en-US/docs/Web/API/Response
 */

/**
 * @callback matcher
 * @param {string} url url sent to fetch for the request
 * @param {Object} opts options for the request object being created by fetch-mock
 *
 * @return {boolean} true if the request should be matched.
 */

/**
 * Mocks out request to prpc service for a given fetchMock.
 *
 * @param {Object} fetchMock instance to use for mocking.
 * @param {string} service is the string prpc service we wish to call.
 * @param {string} rpc string that service we want to call.
 * @param {(fn|Object)} data data can be either a function or a callback which produces data.
 * @param {(matcher|undefined)} matcher is a predicate which determines whether the request should be matched.
 * @param {boolean} overwriteRoutes
 */
export function mockPrpc(
  fetchMock,
  service,
  rpc,
  data,
  matcher = null,
  overwriteRoutes = undefined
) {
  let response = (_url, _opts) =>
    new Response(stringify(data), {
      status: 200,
      headers: prpcHeaders,
    });
  if (typeof data === "function") {
    response = (_url, opts) => {
      const body = JSON.parse(opts.body);
      return new Response(stringify(data(body)), {
        status: 200,
        headers: prpcHeaders,
      });
    };
  }
  if (matcher) {
    const matchingFn = (url, opts) => {
      return (
        url.endsWith(`${service}/${rpc}`) &&
        opts.method.toLowerCase() === "post" &&
        matcher(JSON.parse(opts.body))
      );
    };
    fetchMock.mock(matchingFn, response, { overwriteRoutes });
  } else {
    fetchMock.post(`path:/prpc/${service}/${rpc}`, response, {
      overwriteRoutes,
    });
  }
}

/**
 * Makes a pRPC request always return unauthorized - which is a 403 status.
 *
 * @param {Object} fetchMock module to apply this too.
 * @param {string} service is the prpc service to mock.
 * @param {string} rpc is the specific RPC call to mock.
 **/
export function mockUnauthorizedPrpc(fetchMock, service, rpc) {
  fetchMock.post(
    `path:/prpc/${service}/${rpc}`,
    (_url, _opts) => {
      return new Response(`)]}'"403 Unauthorized"`, {
        status: 403,
        headers: {
          "x-prpc-grpc-code": "7",
          "content-type": "application/json",
        },
      });
    },
    {
      overwriteRoutes: true,
    }
  );
}

// Add an event listener which will fire after everything is done on the
// element. This one can be stacked on top of others.
export function eventually(ele, callback) {
  ele.addEventListener("busy-end", (_e) => {
    callback(ele);
  });
}

/**
 * Recursively checks whether two values are equal.
 * Not best performance and limited by call stack depth but we are using this
 * primarily for testing so it should be fine.
 **/
export function deepEquals(obja, objb) {
  // If we have two equal primitives, one (or both) these two will be true
  // Use === to ensure differing types are not equal.
  if (obja === objb) {
    return true;
  }
  // Check if we have two arrays of equal length
  if (
    Array.isArray(obja) &&
    Array.isArray(objb) &&
    obja.length === objb.length
  ) {
    // compare each item in array recursively
    for (let i = 0; i < obja.length; i++)
      if (!deepEquals(obja[i], objb[i])) return false;
    return true;
  }
  // Check if we have two objects, recurse though
  if (Object(obja) === obja && Object(objb) === objb) {
    const keya = Object.keys(obja);
    const keyb = Object.keys(objb);
    if (keya.length !== keyb.length) return false;
    for (const key of keya) if (!deepEquals(obja[key], objb[key])) return false;
    return true;
  }

  return false;
}

/**
 * Creates a function which will match a `fetchMock` call to a pRPC
 * rpc request.
 */
export function createRequestFilter(service, rpc, data) {
  return (call) => {
    if (!call[0].endsWith(`${service}/${rpc}`)) return false;
    if (typeof data === "undefined") return true;
    const request = JSON.parse(call[1].body);
    return deepEquals(request, data);
  };
}
