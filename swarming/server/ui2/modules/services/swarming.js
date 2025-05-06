// Copyright 2023 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import { PrpcService } from "./common";

export class SwarmingService extends PrpcService {
  get service() {
    return "swarming.v2.Swarming";
  }

  /**
   * Request and response map to the GetPermissions RPC described here:
   * https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#108
   */
  permissions(request) {
    return this._call("GetPermissions", request);
  }

  /**
   * Sends a server details request to the server. See GetDetails rpc for more details.
   * https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#101
   */
  details() {
    return this._call("GetDetails", {});
  }

  /**
   * Requests a new bootstrap token for admins. See GetToken rpc for more details.
   * https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#106
   **/
  token() {
    return this._call("GetToken", {});
  }
}
