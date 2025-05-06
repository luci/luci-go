// Copyright 2023 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import { PrpcClient } from "@chopsui/prpc-client";

export class PrpcService {
  /**
   * @param {string} accessToken - bearer token to use to authenticate requests.
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
    const additionalHeaders = {
      authorization: this._token,
    };
    return this._client.call(this.service, method, request, additionalHeaders);
  }
}
