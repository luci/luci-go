// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import "./index.js";

import { bots10, fleetCount, fleetDimensions, queryCount } from "./test_data";
import { requireLogin, mockAuthorizedSwarmingService } from "../test_util";
import fetchMock from "fetch-mock";

(function () {
  // Can't use import fetch-mock because the library isn't quite set up
  // correctly for it, and we get strange errors about 'this' not being defined.

  // uncomment to stress test with 5120 items
  // bots_10.items.push(...bots_10.items);
  // bots_10.items.push(...bots_10.items);
  // bots_10.items.push(...bots_10.items);
  // bots_10.items.push(...bots_10.items);
  // bots_10.items.push(...bots_10.items);
  // bots_10.items.push(...bots_10.items);
  // bots_10.items.push(...bots_10.items);
  // bots_10.items.push(...bots_10.items);
  // bots_10.items.push(...bots_10.items);

  mockAuthorizedSwarmingService(fetchMock, {
    delete_bot: true,
    list_bots: ["Chrome"],
  });

  fetchMock.get(
    "glob:/_ah/api/swarming/v1/bots/list?*",
    requireLogin(bots10, 500)
  );

  fetchMock.get(
    "glob:/_ah/api/swarming/v1/bots/dimensions?*",
    requireLogin(fleetDimensions, 400)
  );

  fetchMock.get("/_ah/api/swarming/v1/bots/count", requireLogin(fleetCount));
  fetchMock.get(
    "glob:/_ah/api/swarming/v1/bots/count?*",
    requireLogin(queryCount, 100)
  );

  fetchMock.post(
    "glob:/_ah/api/swarming/v1/bot/*/delete",
    requireLogin(200, 750)
  );

  // Everything else
  fetchMock.catch(404);

  // autologin for ease of testing locally - comment this out if using the real flow.
  document.querySelector("oauth-login")._logIn();
})();
