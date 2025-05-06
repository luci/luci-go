// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import "./index.js";
import { mockAuthorizedSwarmingService, mockPrpc } from "../test_util";
import fetchMock from "fetch-mock";

(function () {
  mockAuthorizedSwarmingService(fetchMock, {
    getBootstrapToken: true,
  });

  const loggedInToken = {
    bootstrapToken: "8675309JennyDontChangeYourNumber8675309",
  };
  mockPrpc(fetchMock, "swarming.v2.Swarming", "GetToken", loggedInToken);
  // Everything else
  fetchMock.catch(404);
})();
