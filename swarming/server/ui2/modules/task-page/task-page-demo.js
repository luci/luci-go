// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.
import "./index.js";

import { taskOutput, taskResult, taskRequest } from "./test_data";
import { requireLogin, mockAuthorizedSwarmingService } from "../test_util";
import { $$ } from "common-sk/modules/dom";
import fetchMock from "fetch-mock";

(function () {
  const PERF_TEST_LOGS = true;

  mockAuthorizedSwarmingService(fetchMock, {});

  fetchMock.get(
    "glob:/_ah/api/swarming/v1/server/permissions?task_id=*",
    requireLogin({
      cancel_task: true,
    })
  );

  fetchMock.get(
    "glob:/_ah/api/swarming/v1/task/*/request",
    requireLogin(taskRequest, 100)
  );

  fetchMock.get(
    "glob:/_ah/api/swarming/v1/task/*/result?include_performance_stats=true",
    requireLogin(taskResult, 200)
  );

  fetchMock.get(
    "glob:/_ah/api/swarming/v1/task/*/result",
    requireLogin(taskResult, 600)
  );

  if (PERF_TEST_LOGS) {
    // generate some logs
    const PAGE_LENGTH = 100 * 1024;
    const largeLogs = [];
    const ALPHABET =
      "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789";
    let nextbit = "";
    for (let i = 0; i < 20; i++) {
      let thisStr = nextbit;
      while (thisStr.length < PAGE_LENGTH) {
        thisStr += " ";
        thisStr += randomWord();

        // 1% chance to do a line break
        if (Math.random() < 0.01) {
          thisStr += "\n";
        }
      }
      nextbit = thisStr.substring(PAGE_LENGTH);
      thisStr = thisStr.substring(0, PAGE_LENGTH);
      largeLogs.push(thisStr);
    }
    largeLogs.push(nextbit + "\nEND OF LOGS");

    function randomWord() {
      const length = randomInt(1, 10) + randomInt(0, 4);
      let str = "";
      for (let i = 0; i < length; i++) {
        str += ALPHABET[Math.floor(Math.random() * ALPHABET.length)];
      }
      return str;
    }

    const start = Date.now();

    let stdoutCounter = 0;
    fetchMock.get(
      "glob:/_ah/api/swarming/v1/task/*/stdout*",
      requireLogin((url, opts) => {
        if (stdoutCounter === 0) {
          stdoutCounter++;
          return {
            output: "",
            state: "PENDING",
          };
        }
        if (stdoutCounter < largeLogs.length) {
          stdoutCounter++;
          if (stdoutCounter >= largeLogs.length) {
            const end = Date.now();
            console.log(`Took ${(end - start) / 1000} seconds`);
          }
          return {
            output: largeLogs[stdoutCounter - 1],
            state: "RUNNING",
          };
        }
        return {
          output: "",
          state: "COMPLETED",
        };
      }, 100)
    );
  } else {
    let stdoutCounter = 1; // put at 1 so in demo we don't have to wait
    fetchMock.get(
      "glob:/_ah/api/swarming/v1/task/*/stdout*",
      requireLogin((url, opts) => {
        // Return pending and '',
        // running and partial content
        // stopped and remaining content
        switch (stdoutCounter) {
          case 0:
            stdoutCounter++;
            return {
              output: "",
              state: "PENDING",
            };
          case 1:
            stdoutCounter++;
            return {
              output: taskOutput.substring(0, 100),
              state: "RUNNING",
            };
          case 2:
            stdoutCounter++;
            return {
              output: taskOutput.substring(100, 300),
              state: "RUNNING",
            };
          case 3:
            stdoutCounter = 1; // skip pending for faster local dev
            // set to 0 for fuller testing
            return {
              output: taskOutput.substring(300),
              state: "COMPLETED",
            };
        }
      }, 800)
    );
  }

  function randomInt(min, max) {
    return Math.floor(Math.random() * (max - min) + min);
  }

  function randomBotCounts() {
    const total = randomInt(10, 200);
    return {
      busy: Math.floor(total * 0.84),
      count: total,
      dead: randomInt(0, total * 0.1),
      quarantined: randomInt(1, total * 0.1),
      maintenance: randomInt(0, total * 0.1),
    };
  }
  fetchMock.get(
    "glob:/_ah/api/swarming/v1/bots/count?*",
    requireLogin(randomBotCounts, 300)
  );

  function randomTaskCounts() {
    return {
      count: randomInt(10, 2000),
    };
  }
  fetchMock.get(
    "glob:/_ah/api/swarming/v1/tasks/count?*",
    requireLogin(randomTaskCounts, 300)
  );

  fetchMock.post(
    "/_ah/api/swarming/v1/tasks/new",
    requireLogin({ task_id: "testid002" }, 800)
  );

  fetchMock.post(
    "glob:/_ah/api/swarming/v1/task/*/cancel",
    requireLogin({ success: true }, 200)
  );

  // Everything else
  fetchMock.catch(404);

  const ele = $$("task-page");
  if (!ele._taskId) {
    ele._taskId = "testid000";
  }
  ele._logFetchPeriod = 500;

  // autologin for ease of testing locally - comment this out if using the real flow.
  $$("oauth-login")._logIn();
})();
