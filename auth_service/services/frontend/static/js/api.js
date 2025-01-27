// Copyright 2021 The LUCI Authors.
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

var api = (function () {
  'use strict';

  var exports = {};

  //// pRPC support.

  // A table of gRPC codes from google/rpc/code.proto.
  const GRPC_CODES = {
    OK: 0,
    CANCELLED: 1,
    UNKNOWN: 2,
    INVALID_ARGUMENT: 3,
    DEADLINE_EXCEEDED: 4,
    NOT_FOUND: 5,
    ALREADY_EXISTS: 6,
    PERMISSION_DENIED: 7,
    RESOURCE_EXHAUSTED: 8,
    FAILED_PRECONDITION: 9,
    ABORTED: 10,
    OUT_OF_RANGE: 11,
    UNIMPLEMENTED: 12,
    INTERNAL: 13,
    UNAVAILABLE: 14,
    DATA_LOSS: 15,
    UNAUTHENTICATED: 16,
  };
  exports.GRPC_CODES = GRPC_CODES;

  function codeToStr(code) {
    for (const [key, value] of Object.entries(GRPC_CODES)) {
      if (code == value) {
        return key;
      }
    }
    return 'CODE_' + code;
  };

  // CallError is thrown by `call` on unsuccessful responses.
  //
  // It contains the gRPC code and the error message.
  class CallError extends Error {
    constructor(code, error) {
      super(`pRPC call failed with code ${codeToStr(code)}: ${error}`);
      this.name = 'CallError';
      this.code = code;
      this.error = error;
    }
  };
  exports.CallError = CallError;

  // Calls a pRPC method.
  //
  // Args:
  //   service: the full service name e.g. "auth.service.Accounts".
  //   method: a pRPC method name e.g. "GetSelf".
  //   request: a dict with JSON request body.
  //
  // Returns:
  //   The response as a JSON dict.
  //
  // Throws:
  //   CallError (both on network issues and non-OK responses).
  async function call(service, method, request) {
    try {
      // See https://pkg.go.dev/go.chromium.org/luci/grpc/prpc for definition of
      // the pRPC protocol (in particular its JSON encoding).
      let resp = await fetch('/prpc/' + service + '/' + method, {
        method: 'POST',
        headers: {
          'Accept': 'application/prpc; encoding=json',
          'Content-Type': 'application/prpc; encoding=json',
          'X-Xsrf-Token': xsrf_token,
        },
        credentials: 'same-origin',
        cache: 'no-cache',
        redirect: 'error',
        referrerPolicy: 'no-referrer',
        body: JSON.stringify(request || {}),
      });
      let body = await resp.text();

      // Valid pRPC responses must have 'X-Prpc-Grpc-Code' header with an
      // integer code. The only exception is >=500 HTTP status replies. They are
      // internal errors that can be thrown even before the pRPC server is
      // reached.
      let code = parseInt(resp.headers.get('X-Prpc-Grpc-Code'));
      if (code == NaN) {
        if (resp.status >= 500) {
          throw new CallError(GRPC_CODES.INTERNAL, body);
        }
        throw new CallError(
          GRPC_CODES.INTERNAL,
          'no valid X-Prpc-Grpc-Code in the response',
        );
      }

      // Non-OK responses have the error message as their body.
      if (code != GRPC_CODES.OK) {
        throw new CallError(code, body);
      }

      // OK responses start with `)]}'\n`, followed by the JSON response body.
      const prefix = ')]}\'\n';
      if (!body.startsWith(prefix)) {
        throw new CallError(
          GRPC_CODES.INTERNAL,
          'missing the expect JSON response prefix',
        );
      }

      return JSON.parse(body.slice(prefix.length));
    } catch (err) {
      if (err instanceof CallError) {
        throw err;
      } else {
        throw new CallError(GRPC_CODES.INTERNAL, err.message);
      }
    }
  };
  exports.call = call;


  //// API calls.

  // Get all groups.
  exports.groups = function (requireFresh) {
    return call('auth.service.Groups', 'ListGroups', { 'fresh': requireFresh });
  };

  // Get individual group.
  exports.groupRead = function (request) {
    return call('auth.service.Groups', 'GetGroup', { 'name': request });
  };

  // Get the exhaustive membership listing of an individual group.
  exports.groupExpand = function (name) {
    return call('auth.service.Groups', 'GetExpandedGroup', { 'name': name });
  };

  // Delete individual group.
  exports.groupDelete = function (name, etag) {
    return call('auth.service.Groups', 'DeleteGroup', { 'name': name, 'etag': etag });
  };

  // Create a group.
  exports.groupCreate = function (authGroup) {
    return call('auth.service.Groups', 'CreateGroup', { 'group': authGroup });
  }

  // Update a group.
  exports.groupUpdate = function (authGroup) {
    return call('auth.service.Groups', 'UpdateGroup', { 'group': authGroup });
  }

  // Find the groups a principal (user or group) is in.
  exports.groupLookup = function (principal) {
    return call('auth.service.Groups', 'GetSubgraph', { 'principal': principal });
  }

  // Get all allowlists.
  exports.ipAllowlists = function () {
    return call('auth.service.Allowlists', 'ListAllowlists');
  };

  // Get all changeLogs.
  exports.changeLogs = function (target, revision, pageSize, pageToken) {
    var q = {};
    if (target) {
      q.target = target;
    }
    if (revision) {
      q.authDbRev = revision;
    }
    if (pageSize) {
      q.pageSize = pageSize;
    }
    if (pageToken) {
      q.pageToken = pageToken;
    }

    return call('auth.service.ChangeLogs', 'ListChangeLogs', q)
  }

  // Get all services linked via legacy replica method.
  exports.listReplicas = function () {
    return call('auth.service.Replicas', 'ListReplicas');
  }

  //// XSRF token utilities.


  // The current known value of the XSRF token.
  var xsrf_token = null;

  // Sets the XSRF token.
  exports.setXSRFToken = function (token) {
    xsrf_token = token;
  };

  // Enables the XSRF token refresh timer (firing once an hour).
  exports.startXSRFTokenAutoupdate = function () {
    setInterval(() => {
      call('auth.internals.Internals', 'RefreshXSRFToken', {
        'xsrfToken': xsrf_token,
      }).then(resp => {
        xsrf_token = resp.xsrfToken;
      });
    }, 3600 * 1000);
  };

  return exports;
}());
