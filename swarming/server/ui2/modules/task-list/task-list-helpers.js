// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

/** @module swarming-ui/modules/task-list */
// This file contains a large portion of the JS logic of task-list.
// By keeping JS logic here, the functions can more easily be unit tested
// and it declutters the main task-list.js.
// If a function doesn't refer to 'this', it should go here, otherwise
// it should go inside the element declaration.

// query.fromObject is more readable than just 'fromObject'
import { applyAlias } from "../alias";
import { html } from "lit-html";
import naturalSort from "javascript-natural-sort/naturalSort";
import {
  botPageLink,
  compareWithFixedOrder,
  humanDuration,
  sanitizeAndHumanizeTime,
  taskPageLink,
  toCamelCase,
} from "../util";
import { EXCEPTIONAL_STATES, FILTER_STATES, ONGOING_STATES } from "../task";

const DIMENSIONS_DENYLIST = ["quarantined", "error"];
const EMPTY_VAL = "--";

/** appendPossibleColumns takes the given data and derives columns
 *  that could be displayed. There are two possible data sources:
 *    - the list of dimensions seen on bots from the server
 *    - a map of tags seen on the lists so far
 *  There is also a list of 'hard coded' columns that should be
 *  viable (see possibleColumns) which is added if it does not exist.
 *
 * @param {Object} possibleColumns - Existing set (i.e. String->bool) of
 *        columns so far. New data will be appended to this.
 * @param {Object|Array} data - one of two options listed above.
 */
export function appendPossibleColumns(possibleColumns, data) {
  // Use name as a sentinel value
  if (!possibleColumns["name"]) {
    for (const col of extraKeys) {
      possibleColumns[col] = true;
    }
  }
  if (Array.isArray(data)) {
    // we have a list of dimensions, which are {key: String, value: Array}
    for (const dim of data) {
      if (DIMENSIONS_DENYLIST.indexOf(dim.key) === -1) {
        possibleColumns[dim.key + "-tag"] = true;
      }
    }
  } else {
    // data is a map of tag -> values
    for (const tag in data) {
      if (data.hasOwnProperty(tag)) {
        possibleColumns[tag + "-tag"] = true;
      }
    }
  }
}

/** appendPrimaryMap takes the given data and derives the keys that could
 *  be filtered by and and the values that the keys could be.
 * There are two possible data sources:
 *    - the list of dimensions seen on bots from the server
 *    - a map of tags seen on the lists so far
 *  There is also a list of 'hard coded' key-values, which is added if it
 *  doesn't already exist.
 *
 * @param {Object} primaryMap - Maps String->Array<String> of keys and values.
 * @param {Object|Array} data - one of two options listed above.
 *
 */
export function appendPrimaryMap(primaryMap, data) {
  if (!primaryMap["state"]) {
    primaryMap["state"] = FILTER_STATES;
  }
  if (Array.isArray(data)) {
    // we have a list of dimensions, which are {key: String, value: Array}
    for (const dim of data) {
      if (DIMENSIONS_DENYLIST.indexOf(dim.key) === -1) {
        let existing = primaryMap[dim.key + "-tag"];
        for (const value of dim.value) {
          existing = _insertUnique(existing, value);
        }
        primaryMap[dim.key + "-tag"] = existing;
      }
    }
  } else {
    // data is a map of tag -> values
    for (const tag in data) {
      if (data.hasOwnProperty(tag)) {
        let existing = primaryMap[tag + "-tag"];
        for (const value of data[tag]) {
          existing = _insertUnique(existing, value);
        }
        primaryMap[tag + "-tag"] = existing;
      }
    }
  }
}

/** column returns the display-ready value for a column (aka key)
 *  from a task. It requires the entire state (ele) for potentially complicated
 *  data lookups (and also visibility of the 'verbose' setting).
 *  A custom version can be specified in colMap, with the default being
 *  the attribute of task in col or '--'.
 *
 * @param {string} col - The 'key' of the data to pull.
 * @param {Object} task - The task from which to extract data.
 * @param {Object} ele - The entire task-list object, for context.
 *
 * @returns {String} - The requested column, ready for display.
 */
export function column(col, task, ele) {
  if (!task) {
    console.warn("falsey task passed into column");
    return "";
  }
  const c = colMap[col];
  if (c) {
    return c(task, ele);
  }

  col = stripTag(col);
  let tags = task.tagMap[col];
  if (tags) {
    tags = tags.map((t) => applyAlias(t, col));
    if (ele._verbose) {
      return tags.join(" | ");
    }
    return tags[0];
  }
  return task[col] || EMPTY_VAL;
}

/** A list of special rules for filtering client-side (e.g. while waiting
 *  for the server to reply). In practice, this is anything
 *  that is not a tag, since the API only supports filtering by
 *  tags and these values.
 *  This should not be directly used by task-list, but is
 *  exported for testing.
 */
export const specialFilters = {
  state: function (task, s) {
    if (s.startsWith("QUERY_")) {
      s = s.slice("QUERY_".length);
    }
    const state = task.state;
    if (s === state || s === "ALL") {
      return true;
    }
    if (s === "PENDING_RUNNING") {
      return ONGOING_STATES.has(state);
    }
    const failure = task.failure;
    if (s === "COMPLETED_SUCCESS") {
      return state === "COMPLETED" && !failure;
    }
    if (s === "COMPLETED_FAILURE") {
      return state === "COMPLETED" && failure;
    }
    if (s === "DEDUPED") {
      return state === "COMPLETED" && task.dedupedFrom;
    }
  },
};

/** Filters the tasks like they would be filtered from the server
 * @param {Array<String>} filters - e.g. ['alpha:beta']
 * @param {Array<Object>} tasks: the task objects to filter.
 *
 * @returns {Array<Object>} the tasks that match the filters.
 */
export function filterTasks(filters, tasks) {
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
  return tasks.filter((task) => {
    let matches = true;
    for (const filter of parsedFilters) {
      let [key, value] = filter;
      if (specialFilters[key]) {
        matches = matches && specialFilters[key](task, value);
      } else {
        key = stripTag(key);
        // it's a tag,  which is *not* aliased, so we can just
        // do an exact match (reminder, aliasing only happens in column)
        matches = matches && (task.tagMap[key] || []).indexOf(value) !== -1;
      }
    }
    return matches;
  });
}

/** floorSecond rounds the timestamp down to the second (i.e. removes milliseconds).
 *  @param {Number} ts - a milliseconds since epoch value, e.g. Date.now()
 */
export function floorSecond(ts) {
  return Math.round(ts / 1000) * 1000;
}

/** getColHeader returns the human-readable header for a given column.
 */
export function getColHeader(col) {
  if (col && col.endsWith("-tag")) {
    return `${stripTag(col)} (tag)`;
  }
  return colHeaderMap[col] || col;
}

export function humanizePrimaryKey(key) {
  if (key && key.endsWith("-tag")) {
    return `${stripTag(key)} (tag)`;
  }
  if (key === "state") {
    return "state (of task)";
  }
  return key;
}

function _insertUnique(arr, value) {
  // TODO(kjlubick): this could be tuned with binary search.
  if (!arr || !arr.length) {
    return [value];
  }
  if (arr.indexOf(value) !== -1) {
    return arr;
  }
  for (let i = 0; i < arr.length; i++) {
    // We have found where value should go in sorted order.
    if (value < arr[i]) {
      arr.splice(i, 0, value);
      return arr;
    }
  }
  // value must be bigger than all elements, append to end.
  arr.push(value);
  return arr;
}

/** legacyTags goes through the list of key-value filters and
 *  makes sure they all end in -tag. Old links (e.g. from the
 *  Polymer version) might have omitted -tag, but the only valid
 *  filters are ones that have -tag. This makes the system backwards
 *  compatible with old links.
 *  @param {Array<string>} filters - a list of colon-separated key-values.
 *
 *  @return {Array<string>} - the cleaned up filters.
 */
export function legacyTags(filters) {
  return filters.map((filter) => {
    const idx = filter.indexOf(":");
    if (idx < 0) {
      return filter;
    }
    const key = filter.substring(0, idx);
    if (key.endsWith("-tag") || key === "state") {
      // this is fine
      return filter;
    }
    return key + "-tag" + filter.substring(idx);
  });
}

/** listQueryParams returns a query string for the /list API based on the
 *  passed in args.
 *  @param {Array<string>} filters - a list of colon-separated key-values.
 *  @param {Object} extra - Additional filter params: limit, startTime,
 *          endTime, cursor.
 */
export function listQueryParams(filters, extra) {
  const params = {};
  const tags = [];
  for (const f of filters) {
    const split = f.split(":", 1);
    let key = split[0];
    const rest = f.substring(key.length + 1);
    // we use the -tag as a UI thing to differentiate tags
    // from 'magic values' like name.
    key = stripTag(key);
    if (key === "state") {
      if (rest.startsWith("QUERY_")) {
        params["state"] = rest.toUpperCase();
      } else {
        params["state"] = `QUERY_${rest.toUpperCase()}`;
      }
    } else {
      tags.push(key + ":" + rest);
    }
  }
  if (tags.length > 0) {
    params["tags"] = tags;
  }
  params["limit"] = extra.limit;
  if (extra.cursor) {
    params["cursor"] = extra.cursor;
  }
  params["start"] = extra.start;
  params["end"] = extra.end;

  return params;
}

/** processTasks processes the array of tasks from the server and returns it.
 *  The primary goal is to get the data ready for display.
 *  This function builds onto the set of tags seen overall, which is used
 *  for filtering.
 *
 * @param {Array<Object>} arr - The raw tasks objects.
 * @param {Object} existingTags - a map (String->Array<String>) of tags
 *      to values. The values array should be sorted with no duplicates.
 */
export function processTasks(arr, existingTags) {
  if (!arr) {
    return [];
  }
  const now = new Date();

  for (const task of arr) {
    sanitizeAndHumanizeTime(task, TASK_TIMES);
    const tagMap = {};
    task.tags = task.tags || [];
    for (const tag of task.tags) {
      const split = tag.split(":", 1);
      const key = split[0];
      const rest = tag.substring(key.length + 1);
      // tags are free-form, and could be duplicated
      if (!tagMap[key]) {
        tagMap[key] = [rest];
      } else {
        tagMap[key].push(rest);
      }
      existingTags[key] = _insertUnique(existingTags[key], rest);
    }
    task.tagMap = tagMap;

    if (!task.costsUsd || !Array.isArray(task.costsUsd)) {
      task.costsUsd = EMPTY_VAL;
    } else {
      task.costsUsd.forEach(function (c, idx) {
        task.costsUsd[idx] = "$" + c.toFixed(4);
        if (task.state === "RUNNING" && task.startedTs) {
          task.costsUsd[idx] = task.costsUsd[idx] + "*";
        }
      });
    }

    if (task.costSavedUsd) {
      task.costSavedUsd = "-$" + task.costSavedUsd.toFixed(4);
    }

    // Running tasks have no duration set, so we can figure it out.
    if (!task.duration && task.state === "RUNNING" && task.startedTs) {
      task.duration = (now - task.startedTs) / 1000;
    }
    // Make the duration human readable
    task.humanDuration = humanDuration(task.duration);
    if (task.state === "RUNNING" && task.startedTs) {
      task.humanDuration = task.humanDuration + "*";
    }

    // Deduplicated tasks usually have tasks that ended before they were
    // created, so we need to account for that.
    const et = task.startedTs || task.abandonedTs || new Date();
    const deduped = task.createdTs && et < task.createdTs;

    task.pendingTime = undefined;
    if (!deduped && task.createdTs) {
      task.pendingTime = (et - task.createdTs) / 1000;
    }
    task.humanPendingTime = humanDuration(task.pendingTime);
    if (!deduped && task.createdTs && !task.startedTs && !task.abandonedTs) {
      task.humanPendingTime = task.humanPendingTime + "*";
    }
  }
  return arr;
}

// This puts the times in a aesthetically pleasing order, roughly in
// the order everything happened.
const specialColOrder = [
  "name",
  "createdTs",
  "pendingTime",
  "startedTs",
  "duration",
  "completedTs",
  "abandonedTs",
  "modifiedTs",
];
const compareColumns = compareWithFixedOrder(specialColOrder);

/** sortColumns sorts the bot-list columns in mostly alphabetical order. Some
 *  columns (name) go first or are sorted in a fixed order (timestamp ones)
 *  to aid reading.
 *  @param {Array<String>} cols - The columns to sort.
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

/** stripTag removes the '-tag' suffix from a string, if there is one, and
 *  returns what remains. e.g. 'pool-tag' => 'pool'
 */
export function stripTag(s) {
  if (s && s.endsWith("-tag")) {
    return s.substring(0, s.length - 4);
  }
  return s;
}

/** stripTag removes the '-tag' suffix from a filter string, if there is one, and
 *  returns what remains.  e.g. 'pool-tag:Chrome' => 'pool:Chrome'
 */
export function stripTagFromFilter(s) {
  return s.replace("-tag:", ":");
}

/** tagsOnly takes a list of filters in the form foo:bar
 *  and filters out any that are not tags.
 */
export function tagsOnly(filters) {
  const nonTags = Object.keys(specialFilters);
  return filters.filter((f) => {
    for (const nt of nonTags) {
      if (f.startsWith(nt + ":")) {
        return false;
      }
    }
    return true;
  });
}

/** taskClass returns the CSS class for the given task, based on the state
 *  of said task.
 */
export function taskClass(task) {
  const state = column("state", task);
  if (EXCEPTIONAL_STATES.has(state)) {
    return "exception";
  }
  if (state === "BOT_DIED") {
    return "bot_died";
  }
  if (state === "CLIENT_ERROR") {
    return "client_error";
  }
  if (state === "COMPLETED (FAILURE)") {
    return "failed_task";
  }
  if (ONGOING_STATES.has(state)) {
    return "pending_task";
  }
  return "";
}

const naturalSortDims = {
  "cores-tag": true,
  "cpu-tag": true,
  "gpu-tag": true,
  "machine_type-tag": true,
  "os-tag": true,
  "priority-tag": true,
  "python-tag": true,
  "xcode_version-tag": true,
  "zone-tag": true,
};

/** Returns true or false if a key is "special" enough to be sorted
 *  via natural sort. Natural sort is more expensive and shouldn't be
 *  used for large, arbitrary strings. https://crbug.com/927532
 */
export function useNaturalSort(key) {
  return naturalSortDims[key];
}

/** colHeaderMap maps keys to their human readable name.*/
const colHeaderMap = {
  abandonedTs: "Abandoned On",
  completedTs: "Completed On",
  bot: "Bot Assigned",
  costsUsd: "Cost (USD)",
  createdTs: "Created On",
  duration: "Duration",
  name: "Task Name",
  modifiedTs: "Last Modified",
  startedTs: "Started Working On",
  state: "state (of task)",
  user: "Requesting User",
  pendingTime: "Time Spent Pending",
};

const TASK_TIMES = [
  "abandonedTs",
  "completedTs",
  "createdTs",
  "modifiedTs",
  "startedTs",
];

const extraKeys = [
  "name",
  "state",
  "costsUsd",
  "dedupedFrom",
  "duration",
  "pendingTime",
  "serverVersions",
  "bot",
  "exitCode",
  ...TASK_TIMES,
];

const STILL_RUNNING_MSG =
  "An asterisk indicates the task is still running " +
  "and thus the time is dynamic.";

const colMap = {
  abandonedTs: (task) => task.humanized.time.abandonedTs,
  bot: (task) => {
    const id = task.botId;
    if (id) {
      return html`<a target="_blank" rel="noopener" href=${botPageLink(id)}
        >${id}</a
      >`;
    }
    return EMPTY_VAL;
  },
  completedTs: (task) => task.humanized.time.completedTs,
  costsUsd: function (task) {
    if (task.costSavedUsd) {
      return task.costSavedUsd;
    }
    return task.costsUsd;
  },
  createdTs: (task) => task.humanized.time.createdTs,
  duration: (task) => {
    if (task.humanDuration.indexOf("*")) {
      return html`<span title=${STILL_RUNNING_MSG}
        >${task.humanDuration}</span
      >`;
    }
    return task.humanDuration;
  },
  exitCode: (task) => task.exitCode || "--",
  modifiedTs: (task) => task.humanized.time.modifiedTs,
  name: (task, ele) => {
    let name = task.name;
    if (!ele._verbose && task.name.length > 70) {
      name = name.slice(0, 67) + "...";
    }
    return html`<a
      target="_blank"
      rel="noopener"
      title=${task.name}
      href=${taskPageLink(task.taskId)}
      >${name}</a
    >`;
  },
  pendingTime: (task) => {
    if (task.humanPendingTime.indexOf("*")) {
      return html`<span title=${STILL_RUNNING_MSG}
        >${task.humanPendingTime}</span
      >`;
    }
    return task.humanPendingTime;
  },
  sourceRevision: (task) => {
    const r = task.sourceRevision;
    return r.substring(0, 8);
  },
  startedTs: (task) => task.humanized.time.startedTs,
  state: (task) => {
    const state = task.state;
    if (state === "COMPLETED") {
      if (task.failure) {
        return "COMPLETED (FAILURE)";
      }
      if (task.dedupedFrom) {
        return "COMPLETED (DEDUPED)";
      }
      return "COMPLETED (SUCCESS)";
    }
    return state;
  },
};

/** specialSortMap maps keys to their special sort rules, encapsulated in a
 *  function. The function takes in the current sort direction (1 for ascending)
 *  and -1 for descending and both bots and should return a number a la compare.
 */
export const specialSortMap = {
  abandonedTs: sortableTime("abandonedTs"),
  bot: (dir, taskA, taskB) =>
    dir * naturalSort(taskA.botId || "z", taskB.botId || "z"),
  completedTs: sortableTime("completedTs"),
  createdTs: sortableTime("createdTs"),
  duration: sortableDuration("duration"),
  modifiedTs: sortableTime("modifiedTs"),
  name: (dir, taskA, taskB) => dir * naturalSort(taskA.name, taskB.name),
  pendingTime: sortableDuration("pendingTime"),
  startedTs: sortableTime("startedTs"),
};

/** Given a time attribute like 'abandonedTs', sortableTime returns a function
 *  that compares the tasks based on the attribute.  This is used for sorting.
 *
 *  @param {String} attr - a timestamp attribute.
 */
function sortableTime(attr) {
  // sort times based on the string they come with, formatted like
  // '2016-08-16T13:12:40.606300' which sorts correctly.  Locale time
  // (used in the columns), does not.
  return (dir, a, b) => {
    const aCol = a[attr] || "9999";
    const bCol = b[attr] || "9999";

    return dir * (aCol - bCol);
  };
}

/** Given a duration attribute like 'pendingTime', sortableDuration
 *  returns a function that compares the tasks based on the attribute.
 *  This is used for sorting.
 *
 *  @param {String} attr - a duration-like attribute.
 */
function sortableDuration(attr) {
  return (dir, a, b) => {
    const aCol = a[attr] !== undefined ? a[attr] : 1e12;
    const bCol = b[attr] !== undefined ? b[attr] : 1e12;

    return dir * (aCol - bCol);
  };
}

/**
 * Timestamps in url need to be integers for the `stateReflector` to detect
 * changes in state. Further timestamps have traditionally been used in the url.
 * This class wraps millisecond timestamps and allows those to be easily
 * converted to json by the prpc client.
 **/
export class Timestamp {
  constructor(seconds, ms) {
    this._milliseconds = parseInt(seconds || 0) * 1000 + parseInt(ms || 0);
  }

  get seconds() {
    return floorSecond(this._milliseconds);
  }

  get milliseconds() {
    return this._milliseconds;
  }

  get date() {
    return new Date(this.milliseconds);
  }

  static fromMilliseconds(ms) {
    return new Timestamp(0, ms);
  }

  static hoursAgo(hs) {
    const MILLISECONDS_IN_HOUR = 60 * 60 * 1000;
    return new Timestamp(0, Date.now() - hs * MILLISECONDS_IN_HOUR);
  }

  toJSON() {
    return this.date.toJSON();
  }
}

export function convertFromLegacyState(newState) {
  // convert columns which were snake_case to camelCase
  if (newState.c) {
    newState.c = newState.c.map((col) => {
      return toCamelCase(col);
    });
  }

  // convert sort to camelCase
  if (newState.s) {
    newState.s = toCamelCase(newState.s);
  }
}
