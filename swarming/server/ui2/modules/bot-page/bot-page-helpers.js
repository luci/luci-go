// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import { applyAlias } from "../alias";
import {
  botListLink,
  humanDuration,
  sanitizeAndHumanizeTime,
  timeDiffExact,
} from "../util";

/** parseBotData pre-processes any data in the bot data object.
 *  @param {Object} bot - The raw bot object
 */
export function parseBotData(bot) {
  if (!bot) {
    return {};
  }
  sanitizeAndHumanizeTime(bot, BOT_TIMES);
  bot.state = bot.state || "{}";
  bot.state = JSON.parse(bot.state) || {};

  bot.dimensions = bot.dimensions || [];
  for (const dim of bot.dimensions) {
    dim.value.forEach(function (value, i) {
      dim.value[i] = applyAlias(value, dim.key);
    });
  }

  bot.device_list = [];
  const devices = bot.state.devices;
  if (devices) {
    for (const id in devices) {
      if (devices.hasOwnProperty(id)) {
        const device = devices[id];
        device.id = id;
        bot.device_list.push(device);
        let count = 0;
        let total = 0;
        // device.temp is a map of zone: 'value'
        device.temp = device.temp || {};
        for (const t in device.temp) {
          if (device.temp.hasOwnProperty(t)) {
            total += parseFloat(device.temp[t]);
            count++;
          }
        }
        if (count) {
          device.averageTemp = (total / count).toFixed(1);
        } else {
          device.averageTemp = "???";
        }
      }
    }
  }

  return bot;
}

/** parseBotData pre-processes the events to get them ready to display.
 *  @param {Array<Object>} events - The raw event objects
 */
export function parseEvents(events) {
  if (!events) {
    return [];
  }
  for (const event of events) {
    sanitizeAndHumanizeTime(event, ["ts"]);
    event.state = event.state ? JSON.parse(event.state) : {};
  }

  // Sort the most recent events first.
  events.sort((a, b) => {
    return b.ts - a.ts;
  });
  return events;
}

const getField = (obj, options, def) => {
  for (const opt of options) {
    if (obj[opt]) {
      return obj[opt];
    }
  }
  return def;
};

const getStart = (task) => {
  return getField(task, ["startedTs"]);
};

const getEnd = (task) => {
  return getField(
    task,
    ["completedTs", "abandonedTs", "modifiedTs"],
    new Date()
  );
};

/** parseTasks pre-processes the tasks to get them ready to display.
 *  @param {Array<Object>} tasks - The raw task objects
 */
export function parseTasks(tasks) {
  if (!tasks) {
    return [];
  }
  for (const task of tasks) {
    sanitizeAndHumanizeTime(task, TASK_TIMES);
    if (task.duration) {
      // Task is finished
      task.humanDuration = humanDuration(task.duration);
    } else {
      const end = getEnd(task);
      task.humanDuration = timeDiffExact(task.startedTs, end);
      task.duration = (end.getTime() - task.startedTs) / 1000;
    }
    const totalOverhead =
      (task.performanceStats && task.performanceStats.botOverhead) || 0;
    // total_duration includes overhead, to give a better sense of the bot
    // being 'busy', e.g. when uploading isolated outputs.
    task.totalDuration = task.duration + totalOverhead;
    task.humanTotalDuration = humanDuration(task.totalDuration);
    task.total_overhead = totalOverhead;

    task.humanState = task.state || "UNKNOWN";
    if (task.state === "COMPLETED") {
      // use SUCCESS or FAILURE in ambiguous COMPLETED case.
      if (task.failure) {
        task.humanState = "FAILURE";
      } else if (task.state !== "RUNNING") {
        task.humanState = "SUCCESS";
      }
    }
  }
  tasks.sort((a, b) => {
    return getStart(b) - getStart(a);
  });
  return tasks;
}

/** quarantineMessage produces a quarantined message for this bot.
 *  @param {Object} bot - The bot object
 */
export function quarantineMessage(bot) {
  if (bot && bot.quarantined) {
    let msg = bot.state.quarantined;
    // Sometimes, the quarantined message is actually in 'error'.  This
    // happens when the bot code has thrown an exception.
    if (msg === undefined || msg === "true" || msg === true) {
      msg = bot.state && bot.state.error;
    }
    return msg || "True";
  }
  return "";
}

// Hand-picked list of dimensions that can vary a lot machine to machine,
// that is, dimensions that can be 'too unique'.
const dimensionsToStrip = ["id", "caches", "server_version"];

/** siblingBotsLink returns a url to a bot-list that has similar
 *  dimensions to the ones passed in
 *  @param {Array<Object>} dimensions - have 'key' and 'value'. To be
 *                         matched against.
 */
export function siblingBotsLink(dimensions) {
  const cols = ["id", "os", "task", "status"];
  if (!dimensions) {
    return botListLink([], cols);
  }

  dimensions = dimensions.filter((dim) => {
    return dimensionsToStrip.indexOf(dim.key) === -1;
  });

  for (const dim of dimensions) {
    if (cols.indexOf(dim.key) === -1) {
      cols.push(dim.key);
    }
  }

  return botListLink(dimensions, cols);
}

const BOT_TIMES = ["firstSeenTs", "lastSeenTs", "leaseExpirationTs"];
const TASK_TIMES = ["startedTs", "completedTs", "abandonedTs", "modifiedTs"];
