// Copyright 2023 The LUCI Authors.
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

import { PrpcClient } from "@chopsui/prpc-client";

export class PrpcService {
  /**
   * @param {string|function(): string} accessToken - bearer token (or getter)
   *  to use to authenticate requests.
   * @param {AbortController} signal - abort controller provided by caller if
   *  we wish to abort request
   * @param {Object} opts - @chopui/prpc-client PrpcOptions, additional options.
   */
  constructor(accessToken, signal = null, opts = {}) {
    const prpcOpts = {
      ...opts,
      accessToken: undefined,
    };
    // If we are running a live demo, use insecure to avoid ssl errors
    if (window.LIVE_DEMO) {
      prpcOpts.insecure = true;
    }
    this._token = accessToken;
    if (signal) {
      const fetchFn = (url, opts) => {
        opts.signal = signal;
        return fetch(url, opts);
      };
      prpcOpts.fetchImpl = fetchFn;
    }
    this._client = new PrpcClient(prpcOpts);
  }

  get service() {
    throw new Error("Subclasses must define service");
  }

  _call(method, request) {
    const token =
      typeof this._token === "function" ? this._token() : this._token;
    const additionalHeaders = {
      authorization: token,
    };
    return this._client.call(this.service, method, request, additionalHeaders);
  }
}
