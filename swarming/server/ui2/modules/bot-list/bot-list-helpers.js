// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

/** @module swarming-ui/modules/bot-list */
// This file contains a large portion of the JS logic of bot-list.
// By keeping JS logic here, the functions can more easily be unit tested
// and it declutters the main bot-list.js.
// If a function doesn't refer to 'this', it should go here, otherwise
// it should go inside the element declaration.

import * as human from "common-sk/modules/human";
import { html } from "lit-html";
import naturalSort from "javascript-natural-sort/naturalSort";
import {
  compareWithFixedOrder,
  sanitizeAndHumanizeTime,
  taskPageLink,
} from "../util";
import { applyAlias } from "../alias";

const EMPTY_VAL = "--";

/** aggregateTemps looks through the temperature data and computes an
 *  average temp. Beyond that, it prepares the temperature data for
 *  better displaying.
 */
export function aggregateTemps(temps) {
  if (!temps) {
    return {};
  }
  const zones = [];
  let avg = 0;
  for (const k in temps) {
    if (temps.hasOwnProperty(k)) {
      zones.push(k + ": " + temps[k]);
      avg += +temps[k];
    }
  }
  avg = avg / zones.length;
  if (avg) {
    avg = avg.toFixed(1);
  } else {
    avg = "unknown";
  }
  return {
    average: avg,
    zones: zones.join(" | ") || "unknown",
  };
}

/** attribute looks first in dimension and then in state for the
 * specified attribute. This will always return an array. If there is
 * no matching attribute, ['UNKNOWN'] will be returned. The typical
 * caller of this is column(), so the return value is generally a string.
 *
 * @param {Object} bot - The bot from which to extract data.
 * @param {string} attr - The 'key' of the data to pull.
 * @param {Object|string} none - (Optional) a none value
 *
 * @returns {String} - The requested attribute, potentially ready for display.
 */
export function attribute(bot, attr, none) {
  none = none || "UNKNOWN";
  return fromDimension(bot, attr) || fromState(bot, attr) || [none];
}

/** botLink creates the href attribute for linking to a single bot.*/
export function botLink(botId) {
  return `/bot?id=${botId}`;
}

/** column returns the display-ready value for a column (aka key)
 *  from a bot. It requires the entire state for potentially complicated
 *  data lookups (and also visibility of the 'verbose' setting).
 *  A custom version can be specified in colMap, with the default being
 *  The longest (assumed to be most specific) item returned from
 *  attribute()).
 *
 * @param {string} col - The 'key' of the data to pull.
 * @param {Object} bot - The bot from which to extract data.
 * @param {Object} ele - The entire bot-list object, for context.
 *
 * @returns {String} - The requested column, ready for display.
 */
export function column(col, bot, ele) {
  if (!bot) {
    console.warn("falsey bot passed into column");
    return "";
  }
  const c = colMap[col];
  if (c) {
    return c(bot, ele);
  }
  let emptyVal = EMPTY_VAL;
  if (noneDimensions.indexOf(col) !== -1) {
    emptyVal = "none";
  }
  const values = attribute(bot, col, emptyVal).map((v) => applyAlias(v, col));
  return longestOrAll(values, ele._verbose);
}

/** devices returns a potentially empty list of devices (e.g. Android devices)
 *  that are on this machine. This will generally be length 1 or 0 (although)
 *  the UI has some support for multiple devices.
 */
export function devices(bot) {
  return bot.state.devices || [];
}

// A list of special rules for filters. In practice, this is anything
// that is not a dimension, since the API only supports filtering by
// dimensions and these values.
export const specialFilters = {
  id: function (bot, id) {
    return bot.botId === id;
  },
  status: function (bot, status) {
    if (status === "quarantined") {
      return bot.quarantined;
    } else if (status === "maintenance") {
      return !!bot.maintenanceMsg;
    } else if (status === "dead") {
      return bot.isDead;
    } else {
      // Status must be 'alive'.
      return !bot.isDead;
    }
  },
  task: function (bot, task) {
    if (task === "idle") {
      return !bot.taskId;
    }
    // Task must be 'busy'.
    return !!bot.taskId;
  },
};

/** dimensionsOnly takes a list of filters in the form foo:bar
 *  and filters out any that are not dimensions.
 */
export function dimensionsOnly(filters) {
  const nonDimensions = Object.keys(specialFilters);
  return filters.filter((f) => {
    for (const nd of nonDimensions) {
      if (f.startsWith(nd + ":")) {
        return false;
      }
    }
    return true;
  });
}

/** Filters the bots like they would be filtered from the server
 * @param {Array<String>} filters - e.g. ['alpha:beta']
 * @param {Array<Object>} bots - the bot objects to filter.
 *
 * @returns {Array<Object>} the bots that match the filters.
 */
export function filterBots(filters, bots) {
  const parsedFilters = [];
  // Preprocess the filters
  for (const filterString of filters) {
    const idx = filterString.indexOf(":");
    const key = filterString.slice(0, idx);
    const value = filterString.slice(idx + 1);
    parsedFilters.push([key, value]);
  }
  // apply the filters in an AND way, that is, it must
  // match all the filters
  return bots.filter((bot) => {
    let matches = true;
    for (const filter of parsedFilters) {
      const [key, value] = filter;
      if (specialFilters[key]) {
        matches &= specialFilters[key](bot, value);
      } else {
        // it's a dimension, which is *not* aliased, so we can just
        // do an exact match (reminder, aliasing only happens in column);
        matches &= attribute(bot, key, []).indexOf(value) !== -1;
      }
    }
    return matches;
  });
}

/** fromDimensions returns the array of dimension values that match the given
 *  key or null if this bot doesn't have that dimension.
 * @param {Object} bot - The bot from which to extract data.
 * @param {string} dim - The 'key' of the dimension to look for.
 */
export function fromDimension(bot, dim) {
  if (!bot || !bot.dimensions || !dim) {
    return null;
  }
  for (let i = 0; i < bot.dimensions.length; i++) {
    if (bot.dimensions[i].key === dim) {
      return bot.dimensions[i].value;
    }
  }
  return null;
}

/** fromState returns the array of values that match the given key
 *  from a bot's state or null if this bot doesn't have it..
 * @param {Object} bot - The bot from which to extract data.
 * @param {string} attr - The 'key' of the state data to look for.
 */
export function fromState(bot, attr) {
  if (!bot || !bot.state || !bot.state[attr]) {
    return null;
  }
  const state = bot.state[attr];
  if (Array.isArray(state)) {
    return state;
  }
  return [state];
}

/** getColHeader returns the human-readable header for a given column.
 */
export function getColHeader(col) {
  return colHeaderMap[col] || col;
}

// The list of things we do count data for, in the order they are presented.
const countTypes = [
  "All",
  "Alive",
  "Busy",
  "Idle",
  "Dead",
  "Quarantined",
  "Maintenance",
];

/** initCounts creates the default list of objects for displaying counts.
 */
export function initCounts() {
  return countTypes.map((label) => {
    return { label: label, key: "" };
  });
}

/** listQueryParams returns a query string for the /list API based on the
 *  passed in args.
 *  @param {Array<string>} filters - a list of colon-separated key-values.
 *  @param {Number} limit - the limit of results to return.
 *  @param {String} cursor - An optional cursor for server pagination.
 */
export function listQueryParams(filters, limit, cursor) {
  const params = {};
  const dims = [];
  for (const f of filters) {
    const split = f.split(":", 1);
    const col = split[0];
    const rest = f.substring(col.length + 1);
    if (col === "status") {
      if (rest === "alive") {
        params.isDead = "FALSE";
      } else if (rest === "quarantined") {
        params.quarantined = "TRUE";
      } else if (rest === "maintenance") {
        params.inMaintenance = "TRUE";
      } else if (rest === "dead") {
        params.isDead = "TRUE";
      }
    } else if (col === "task") {
      if (rest === "busy") {
        params.isBusy = "TRUE";
      } else if (rest === "idle") {
        params.isBusy = "FALSE";
      }
    } else {
      // We can assume dimension here. The only other possibility
      // is that a user has changed their filters w/o using the UI
      // (which checks proper dimensions) and garbage in == garbage out.
      dims.push({ key: col, value: rest });
    }
  }
  params.dimensions = dims;
  params.limit = limit;
  if (cursor) {
    params.cursor = cursor;
  }
  return params;
}

/** longestOrAll returns the longest (by string length) value of the array
 *  or all the values of the array joined with | if verbose is true.
 *  @param {Array<string>} arr - Any list of string values.
 *  @param {Boolean} verbose - If all the values should be returned.
 *  @return {string} the longest value.
 */
export function longestOrAll(arr, verbose) {
  if (verbose) {
    return arr.join(" | ");
  }
  let most = "";
  for (let i = 0; i < arr.length; i++) {
    if (arr[i] && arr[i].length > most.length) {
      most = arr[i];
    }
  }
  return most;
}

/** makePossibleColumns processes the array of dimensions from the server
 *  and returns it. The primary objective is to remove denied
 *  dimensions and make sure any are there that the server doesn't provide.
 *  This will be turned into possible columns
 */
export function makePossibleColumns(arr) {
  if (!arr) {
    return [];
  }
  const dims = [];
  arr.forEach(function (d) {
    if (dimensionsDenylist.indexOf(d.key) === -1) {
      dims.push(d.key);
    }
  });
  // Make sure 'id' is in there, but not duplicated (see dimensionsDenylist)
  dims.push("id");
  Array.prototype.push.apply(dims, extraKeys);
  dims.sort();
  return dims;
}

const BOT_TIMES = ["firstSeenTs", "lastSeenTs", "leaseExpirationTs"];

/** processBots processes the array of bots from the server and returns it.
 *  The primary goal is to get the data ready for display.
 *
 * @param cols Array<Object> The raw bots objects.
 */
export function processBots(arr) {
  if (!arr) {
    return [];
  }
  for (const bot of arr) {
    bot.state = (bot.state && JSON.parse(bot.state)) || {};
    sanitizeAndHumanizeTime(bot, BOT_TIMES);
    // get the disks in an easier to deal with format, sorted by size.
    const disks = bot.state.disks || {};
    const keys = Object.keys(disks);
    if (!keys.length) {
      bot.disks = [{ id: "unknown", mb: 0 }];
    } else {
      bot.disks = [];
      for (let i = 0; i < keys.length; i++) {
        bot.disks.push({ id: keys[i], mb: disks[keys[i]].free_mb });
      }
      // Sort these so the biggest disk comes first.
      bot.disks.sort(function (a, b) {
        return b.mb - a.mb;
      });
    }

    // Make sure every bot has a state.temp object and precompute
    // average and list of temps by zone if applicable.
    bot.state.temp = aggregateTemps(bot.state.temp);

    const devices = [];
    const d = (bot && bot.state && bot.state.devices) || {};
    // state.devices is like {Serial:Object}, so we need to keep the serial
    for (const key in d) {
      if (d.hasOwnProperty(key)) {
        const o = d[key];
        o.serial = key;
        o.okay = o.state === "available";
        // It is easier to assume all devices on a bot are of the same type
        // than to pick through the (incomplete) device state and find it.
        // Bots that are quarantined because they have no devices
        // still have devices in their state (the last known device attached)
        // but don't have the device_type dimension. In that case, we punt
        // on device type.
        const types = fromDimension(bot, "device_type") || ["UNKNOWN"];
        o.device_type = types[0];
        o.temp = aggregateTemps(o.temp);
        devices.push(o);
      }
    }
    // For determinism, sort by device id
    devices.sort((a, b) => {
      // Don't use natural sort because that can confusingly put
      // 89ABCDEF012 before 3456789ABC
      if (a.serial < b.serial) {
        return -1;
      } else if (a.serial > b.serial) {
        return 1;
      }
      return 0;
    });
    bot.state.devices = devices;
  }

  return arr;
}

/** processCounts picks the data from the passed in JSON and feeds it into
 *  the passed in array of objects (see initCounts).
 */
export function processCounts(output, countJSON) {
  // output is expected to be in the order described by countTypes.
  output[0].value = parseInt(countJSON.count || 0); // All
  output[1].value =
    parseInt(countJSON.count || 0) - parseInt(countJSON.dead || 0); // Alive
  output[2].value = parseInt(countJSON.busy || 0); // Busy
  output[3].value =
    parseInt(countJSON.count || 0) - parseInt(countJSON.busy || 0); // Idle
  output[4].value = parseInt(countJSON.dead || 0); // Dead
  output[5].value = parseInt(countJSON.quarantined || 0); // Quarantined
  output[6].value = parseInt(countJSON.maintenance || 0); // Maintenance
  return output;
}

const noneDimensions = ["device_os", "device_type", "gpu"];

/** processPrimaryMap creates a map of primary keys (e.g. left column) based
 *  on dimensions and other interesting options (e.g. device-related things).
 *  The primary keys map to the values they could be filtered by.
 */
export function processPrimaryMap(dimensions) {
  // pMap will have a list of columns to available values (primary key
  // to secondary values). This includes bot dimensions, but also
  // includes state like disk_space, quarantined, busy, etc.
  dimensions = dimensions || [];

  const pMap = {};
  dimensions.forEach(function (d) {
    if (dimensionsDenylist.indexOf(d.key) >= 0) {
      return;
    }
    // value is an array of all seen values for the dimension d.key
    // We keep it unaliased, because aliases will be applied when displayed.
    pMap[d.key] = d.value;
  });

  // Add some options that might not show up.
  pMap["android_devices"] && pMap["android_devices"].push("0");
  for (const key of noneDimensions) {
    if (pMap[key] && pMap[key].indexOf("none") === -1) {
      pMap[key].push("none");
    }
  }

  pMap["id"] = null;

  // Create custom filter/sorting options
  pMap["task"] = ["busy", "idle"];
  pMap["status"] = ["alive", "dead", "quarantined", "maintenance"];

  // No need to sort any of this, bot-filters sorts secondary items
  // automatically, especially when the user types a query.
  return pMap;
}

const specialColOrder = ["id", "task"];
const compareColumns = compareWithFixedOrder(specialColOrder);

/** sortColumns sorts the bot-list columns in mostly alphabetical order. Some
 *  columns (id, task) go first to maintain with behavior from previous
 *  versions.
 *  @param {Array<String>} cols - The columns
 */
export function sortColumns(cols) {
  cols.sort(compareColumns);
}

/** sortPossibleColumns sorts the columns in the column selector. It puts the
 *  selected ones on top in the order they are displayed and the rest below
 *  in alphabetical order.
 *
 * @param {Array<String>} columns - The columns to sort. They will be sorted
 *          in place.
 * @param {Array<String>} selectedCols - The currently selected columns.
 */
export function sortPossibleColumns(columns, selectedCols) {
  const selected = {};
  for (const c of selectedCols) {
    selected[c] = true;
  }

  columns.sort((a, b) => {
    // Show selected columns above non selected columns
    const selA = selected[a];
    const selB = selected[b];
    if (selA && !selB) {
      return -1;
    }
    if (selB && !selA) {
      return 1;
    }
    if (selA && selB) {
      // Both keys are selected, thus we put them in display order.
      return compareColumns(a, b);
    }
    // neither column was selected, fallback to alphabetical sorting.
    return a.localeCompare(b);
  });
}

function timeDiffApprox(date) {
  if (!date) {
    return "eons";
  }
  return human.diffDate(date.getTime());
}

const naturalSortDims = {
  cores: true,
  cpu: true,
  gpu: true,
  "host-cpu": true,
  machineType: true,
  os: true,
  python: true,
  xcode_version: true,
  zone: true,
};

/** Returns true or false if a key is "special" enough to be sorted
 *  via natural sort. Natural sort is more expensive and shouldn't be
 *  used for large, arbitrary strings. https://crbug.com/927532
 */
export function useNaturalSort(key) {
  return naturalSortDims[key];
}

const dimensionsDenylist = ["quarantined", "error", "id"];

/** extraKeys is a list of things we want to be able to sort by or display
 *  that are not dimensions.
.*/
const extraKeys = [
  "task",
  "externalIp",
  "lastSeen",
  "firstSeen",
  "version",
  // Derived
  "disk_space",
  "uptime",
  "running_time",
  "status",
  "internal_ip",
  "battery_level",
  "battery_voltage",
  "battery_temperature",
  "battery_status",
  "battery_health",
  "bot_temperature",
  "device_temperature",
  "serial_number",
];

/** colHeaderMap maps keys to their human readable name.*/
const colHeaderMap = {
  // Returned as part of BotInfo
  id: "Bot Id",
  task: "Current Task",
  externalIp: "External IP",
  firstSeen: "First Seen",
  lastSeen: "Last Seen",
  version: "Client Code Version",
  // Derived from parsing BotInfo.state
  android_devices: "Android Devices",
  battery_health: "Battery Health",
  battery_level: "Battery Level (%)",
  battery_status: "Battery Status",
  battery_temperature: "Battery Temp (°C)",
  battery_voltage: "Battery Voltage (mV)",
  bot_temperature: "Bot Temp (°C)",
  cores: "CPU Core Count",
  cpu: "CPU type",
  device: "Non-android Device",
  device_os: "Device OS",
  device_temperature: "Device Temp (°C)",
  device_type: "Device Type",
  disk_space: "Free Space (MB)",
  gpu: "GPU type",
  internal_ip: "Internal or Local IP",
  os: "OS",
  pool: "Pool",
  running_time: "Swarming Uptime",
  serial_number: "Device Serial Number",
  status: "Status",
  uptime: "Bot Uptime",
  xcode_version: "XCode Version",
};

// Taken from http://developer.android.com/reference/android/os/BatteryManager.html
const BATTERY_HEALTH_ALIASES = {
  1: "Unknown",
  2: "Good",
  3: "Overheated",
  4: "Dead",
  5: "Over Voltage",
  6: "Unspecified Failure",
  7: "Too Cold",
};

const BATTERY_STATUS_ALIASES = {
  1: "Unknown",
  2: "Charging",
  3: "Discharging",
  4: "Not Charging",
  5: "Full",
};

function getStatusSortIndex(bot) {
  if (bot.isDead) return 4;
  if (bot.quarantined) return 3;
  if (bot.maintenanceMsg) return 2;

  // Bot is alive.
  return 1;
}

export const forcedColumns = ["id"];

/** specialSortMap maps keys to their special sort rules, encapsulated in a
 *  function. The function takes in the current sort direction (1 for ascending)
 *  and -1 for descending and both bots and should return a number a la compare.
 */
export const specialSortMap = {
  disk_space: (dir, botA, botB) =>
    dir * naturalSort(botA.disks[0].mb, botB.disks[0].mb),
  id: (dir, botA, botB) => dir * naturalSort(botA.botId, botB.botId),
  firstSeen: (dir, botA, botB) =>
    dir * naturalSort(botA.firstSeenTs, botB.firstSeenTs),
  lastSeen: (dir, botA, botB) =>
    dir * naturalSort(botA.lastSeenTs, botB.lastSeenTs),
  status: (dir, botA, botB) => {
    const statusIndexA = getStatusSortIndex(botA);
    const statusIndexB = getStatusSortIndex(botB);
    if (statusIndexA !== statusIndexB) {
      return dir * (statusIndexA - statusIndexB);
    }

    // Tiebreakers when in bad states are broken by last seen time.
    if (botA.isDead || botA.quarantined || botA.maintenanceMsg) {
      return dir * (botA.lastSeenTs - botB.lastSeenTs);
    }

    // When bots are alive, actually tie, and rely on the
    // behavior of stable sort
    return 0;
  },
  running_time: (dir, botA, botB) =>
    dir *
    naturalSort(
      fromState(botA, "running_time"),
      fromState(botB, "running_time")
    ),
  uptime: (dir, botA, botB) =>
    dir * naturalSort(fromState(botA, "uptime"), fromState(botB, "uptime")),
};

function deviceHelper(callback) {
  return (bot, ele) => {
    const devices = bot.state.devices;
    if (!devices || !devices.length) {
      return "N/A - no devices";
    }
    return devices.map(callback).join(" | ");
  };
}

const colMap = {
  // Derived from BotInfo returned as part of the request.
  version: (bot, ele) => {
    const v = bot.version || "UNKNOWN";
    if (ele._verbose) {
      return v;
    }
    return v.substring(0, 10);
  },
  externalIp: (bot, _ele) => {
    return bot.externalIp || EMPTY_VAL;
  },
  firstSeen: (bot, _ele) => {
    return bot.humanized.time.firstSeenTs;
  },
  id: (bot, _ele) =>
    html`<a target="_blank" rel="noopener" href=${botLink(bot.botId)}
      >${bot.botId}</a
    >`,
  lastSeen: (bot, ele) => {
    if (ele._verbose) {
      return human.localeTime(bot.lastSeenTs);
    }
    return timeDiffApprox(bot.lastSeenTs) + " ago";
  },
  status: (bot, _ele) => {
    if (bot.isDead) {
      return `Dead. Last seen ${human.diffDate(bot.lastSeenTs)} ago`;
    }
    if (bot.quarantined) {
      let msg = fromState(bot, "quarantined");
      if (msg) {
        msg = msg[0];
      }
      // Sometimes, the quarantined message is actually in 'error'.  This
      // happens when the bot code has thrown an exception.
      if (!msg || msg === "true" || msg === true) {
        msg = attribute(bot, "error")[0];
      }
      // Other times, the bot has reported it is quarantined by setting the
      // dimension 'quarantined' to be something.
      if (msg === "UNKNOWN") {
        msg = fromDimension(bot, "quarantined") || "UNKNOWN";
      }
      const deviceStates = [];
      // Show all the errors that are active on devices to make it more
      // clear if this is a transient error (e.g. device is too hot)
      // or if it is requires human interaction (e.g. device is unauthorized)
      devices(bot).forEach(function (d) {
        deviceStates.push(d.state);
      });
      if (deviceStates.length) {
        msg += ` devices: [${deviceStates.join(", ")}]`;
      }
      return `Quarantined: ${msg}`;
    }
    if (bot.maintenanceMsg) {
      return `Maintenance: ${bot.maintenanceMsg}`;
    }
    return "Alive";
  },
  task: (bot, _ele) => {
    if (!bot.taskId) {
      return "idle";
    }
    let id = bot.taskId;
    let mouseover = bot.taskName;
    if (bot.isDead) {
      id = "[died on task]";
      mouseover = `Bot ${bot.botId} was last seen running task ${bot.taskId} (${bot.taskName})`;
    }

    return html`<a
      target="_blank"
      rel="noopener"
      title=${mouseover}
      href=${taskPageLink(bot.taskId)}
      >${id}</a
    >`;
  },
  // Derived from parsing botInfo.state
  android_devices: (bot, ele) => {
    const devs = attribute(bot, "android_devices", "0");
    if (ele._verbose) {
      return devs.join(" | ") + " devices available";
    }
    // max() works on strings as long as they can be coerced to Number.
    return Math.max(...devs) + " devices available";
  },
  battery_health: deviceHelper((device) => {
    const h = (device.battery && device.battery.health) || "UNKNOWN";
    const alias = BATTERY_HEALTH_ALIASES[h] || "";
    return `${alias} (${h})`;
  }),
  battery_level: deviceHelper((device) => {
    return (device.battery && device.battery.level) || "UNKNOWN";
  }),
  battery_status: deviceHelper((device) => {
    const h = (device.battery && device.battery.status) || "UNKNOWN";
    const alias = BATTERY_STATUS_ALIASES[h] || "";
    return `${alias} (${h})`;
  }),
  battery_temperature: deviceHelper((device) => {
    // Battery temps are in tenths of degrees C - convert to more human range.
    return (device.battery && device.battery.temperature / 10) || "UNKNOWN";
  }),
  battery_voltage: deviceHelper((device) => {
    return (device.battery && device.battery.voltage) || "UNKNOWN";
  }),
  bot_temperature: (bot, ele) => {
    if (ele._verbose) {
      return bot.state.temp.zones || "UNKNOWN";
    }
    return bot.state.temp.average || "UNKNOWN";
  },
  device_temperature: (bot, ele) => {
    const devices = bot.state.devices;
    if (!devices || !devices.length) {
      return "N/A - no devices";
    }
    return devices
      .map((device) => {
        if (ele._verbose) {
          return device.temp.zones || UNKNOWN;
        }
        return device.temp.average || UNKNOWN;
      })
      .join(" | ");
  },
  disk_space: (bot, ele) => {
    const aliased = [];
    for (const disk of bot.disks) {
      const alias = human.bytes(disk.mb, human.MB);
      aliased.push(`${disk.id} ${alias} (${disk.mb})`);
    }
    if (ele._verbose) {
      return aliased.join(" | ");
    }
    return aliased[0];
  },
  internal_ip: (bot, _ele) => {
    return attribute(bot, "ip", EMPTY_VAL)[0];
  },
  running_time: (bot, _ele) => {
    const r = fromState(bot, "running_time");
    if (!r) {
      return "UNKNOWN";
    }
    return human.strDuration(r);
  },
  serial_number: deviceHelper((device) => {
    return device.serial || "UNKNOWN";
  }),
  uptime: (bot, _ele) => {
    const u = fromState(bot, "uptime");
    if (!u) {
      return "UNKNOWN";
    }
    return human.strDuration(u);
  },
};

/**
 * Before using the pRPC API, some column names were in snake_case.
 * Converts state of old links to new state available.
 * Ensures that we are backwards compatible with old links.
 *
 * Modifies the state in place.
 **/
export function convertFromLegacyState(state) {
  // Converts old column names to new ones
  const legacyColMapping = {
    last_seen: "lastSeen",
    first_seen: "firstSeen",
    external_ip: "externalIp",
  };
  state.c = (state.c || []).map((col) => {
    const mappedColumn = legacyColMapping[col];
    if (mappedColumn) {
      return mappedColumn;
    }
    return col;
  });
  // Convert legacy sort if necessary
  const mappedSort = legacyColMapping[state.s];
  if (mappedSort) {
    state.s = mappedSort;
  }
}
