// Copyright 2018 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

/** @module swarming-ui/util
 * @description
 *
 * <p>
 *  A general set of useful functions.
 * </p>
 */

import * as human from "common-sk/modules/human";
import * as query from "common-sk/modules/query";
import { upgradeProperty } from "elements-sk/upgradeProperty";

/** botPageLink creates a URL to a given bot */
export function botPageLink(botId) {
  if (!botId) {
    return undefined;
  }
  return "/bot?id=" + botId;
}

/** Create a link to a bot list with the preloaded filters and columns.
 *
 * @param {Array<Object|String>} filters - If Array<Object>, Object
 *     should be {key:String, value:String} or
 *     {key:String, value:Array<String>}. If Array<String>, the Strings
 *     should be valid filters (e.g. 'foo:bar').
 * @param {Array<String>} columns - the column names that should be shown.
 */
export function botListLink(filters = [], columns = []) {
  const fArr = [];
  for (const f of filters) {
    if (f.key && f.value) {
      if (Array.isArray(f.value)) {
        for (const v of f.value) {
          fArr.push(f.key + ":" + v);
        }
      } else {
        fArr.push(f.key + ":" + f.value);
      }
    } else {
      fArr.push(f);
    }
  }
  const obj = {
    f: fArr,
    c: columns,
  };
  return "/botlist?" + query.fromParamSet(obj);
}

/** compareWithFixedOrder returns the sort order of 2 strings. It puts
 * fixedOrder elements first and then sorts the rest alphabetically.
 *
 * @param {Array<String>} fixedOrder - special elements that should be sorted
 *    in the order provided.
 */
export function compareWithFixedOrder(fixedOrder) {
  if (!fixedOrder) {
    fixedOrder = [];
  }
  return function (a, b) {
    let aSpecial = fixedOrder.indexOf(a);
    if (aSpecial === -1) {
      aSpecial = fixedOrder.length + 1;
    }
    let bSpecial = fixedOrder.indexOf(b);
    if (bSpecial === -1) {
      bSpecial = fixedOrder.length + 1;
    }
    if (aSpecial === bSpecial) {
      // Don't need naturalSort since elements shouldn't
      // have numbers as prefixes.
      return a.localeCompare(b);
    }
    // Lower rank in fixedOrder prevails.
    return aSpecial - bSpecial;
  };
}

/** humanDuration formats a duration to be more human readable.
 *
 * @param {Number|String} timeInSecs - The duration to be formatted.
 */
export function humanDuration(timeInSecs) {
  // If the timeInSecs is 0 (e.g. duration of Terminate bot tasks), we
  // still want to display 0s.
  if (timeInSecs === 0 || timeInSecs === "0") {
    return "0s";
  }
  // Otherwise, if timeInSecs is falsey (e.g. undefined), return empty
  // string to reflect that.
  if (!timeInSecs) {
    return "--";
  }
  const ptimeInSecs = parseFloat(timeInSecs);
  // On a bad parse (shouldn't happen), show original.
  if (!ptimeInSecs) {
    return timeInSecs + " seconds";
  }

  // For times greater than a minute, make them human readable
  // e.g. 2h 43m or 13m 42s
  if (ptimeInSecs > 60) {
    return human.strDuration(ptimeInSecs);
  }
  // For times less than a minute, add 10ms resolution.
  return ptimeInSecs.toFixed(2) + "s";
}

/** initPropertyFromAttrOrProperty looks to initialize a property from either
 *  a property or an attribute set on this element.
 *
 * @param {Element} ele -The element.
 * @param {string} prop - The name of the property to initialize.
 * @param {boolean} removeAttr - If the attribute is found, if it should be
 *            removed to avoid stale data.
 *
 */
export function initPropertyFromAttrOrProperty(ele, prop, removeAttr = true) {
  upgradeProperty(ele, prop);
  if (ele[prop] === undefined && ele.hasAttribute(prop)) {
    ele[prop] = ele.getAttribute(prop);
    if (removeAttr) {
      ele.removeAttribute(prop);
    }
  }
}

/** onSmallScreen returns true or false if we are on a "small" screen
 *  where small is arbitrarily chosen (but should include phones).
 */
export function onSmallScreen() {
  return window.innerWidth < 600 || window.innerHeight < 600;
}

/** parseDuration parses a duration string into an integer number of seconds.
 *  e.g:
 *  parseDuration("40s") == 40
 *  parseDuration("2m") == 120
 *  parseDuration("1h") == 3600
 *  parseDuration("foo") == null
 */
export function parseDuration(duration) {
  let number = duration.slice(0, -1);
  if (!/[1-9][0-9]*/.test(number)) {
    return null;
  }
  number = parseInt(number);

  const unit = duration.slice(-1);
  switch (unit) {
    // the fallthroughs here are intentional
    case "h":
      number *= 60;
    case "m":
      number *= 60;
    case "s":
      break;
    default:
      return null;
  }
  return number;
}

/** taskListLink creates a link to a task list with the preloaded
 *  filters and columns.
 *  @param {Array<String|Object> filters - If Array<Object>, Object
 *    should be {key:String, value:String} or
 *    {key:String, value:Array<String>}. If Array<String>, the Strings
 *    should be valid filters (e.g. 'foo:bar').
 *  @param {Array<String>} columns - the column names that should be shown.
 *  @param {Date} start - start time of the list.
 *  @param {Date} end - end time of the list.
 */
export function taskListLink(filters = [], columns = [], start, end) {
  const fArr = [];
  for (const f of filters) {
    if (f.key && f.value) {
      if (Array.isArray(f.value)) {
        for (const v of f.value) {
          fArr.push(f.key + ":" + v);
        }
      } else {
        fArr.push(f.key + ":" + f.value);
      }
    } else {
      fArr.push(f);
    }
  }
  const obj = {
    f: fArr,
    c: columns,
  };

  if (start) {
    obj["st"] = [start.getTime()];
  }
  if (end) {
    obj["et"] = [end.getTime()];
    obj["n"] = [false];
  }

  return "/tasklist?" + query.fromParamSet(obj);
}

/** taskPageLink creates the href attribute for linking to a single task.
 *
 * @param {String} taskId - The full taskID
 * @param {Boolean} disableCanonicalID - For a given task, a canonical task id
 *   will look like 'abcefgh0'. The first try has the id
 *   abcefgh1. If there is a second (transparent retry), it will be
 *   abcefgh2.  We almost always want to link to the canonical one,
 *   because the milo output (if any) will only be generated for
 *   abcefgh0, not abcefgh1 or abcefgh2.
 */
export function taskPageLink(taskId, disableCanonicalID) {
  if (!taskId) {
    return undefined;
  }
  if (!disableCanonicalID) {
    taskId = taskId.substring(0, taskId.length - 1) + "0";
  }
  return `/task?id=${taskId}`;
}

/** timeDiffApprox returns the approximate difference between now and
 *  the specified date.
 */
export function timeDiffApprox(date) {
  if (!date) {
    return "eons";
  }
  return human.diffDate(date.getTime()) || "0s";
}

/** timeDiffExact returns the exact difference between the two specified
 *  dates.  E.g. 2d 22h 22m 28s ago If a second date is not provided,
 *  now is used.
 */
export function timeDiffExact(first, second) {
  if (!first) {
    return "eons";
  }
  if (!second) {
    second = new Date();
  }
  return human.strDuration((second.getTime() - first.getTime()) / 1000) || "0s";
}

/**
 * sets window.LIVE_DEMO = true which allows code to adjust itself to being in a demo environment.
 */
export function setLiveDemoFlag() {
  window.LIVE_DEMO = true;
}

function _base64ToBytes(base64) {
  const binString = atob(base64);
  return Uint8Array.from(binString, (m) => m.codePointAt(0));
}

function _bytesToBase64(bytes) {
  const binString = Array.from(bytes, (x) => String.fromCodePoint(x)).join("");
  return btoa(binString);
}

/**
 * Decodes b64 encoded string to utf8 encoded string.
 *
 * Source: https://developer.mozilla.org/en-US/docs/Glossary/Base64
 **/
export function b64toUtf8(str) {
  return new TextDecoder("utf-8", {
    fatal: false,
  }).decode(_base64ToBytes(str));
}

/**
 * Encodes utf-8 string to base64.
 *
 * Source: https://developer.mozilla.org/en-US/docs/Glossary/Base64
 **/
export function utf8tob64(str) {
  return _bytesToBase64(new TextEncoder().encode(str));
}

const re = /_[a-zA-Z]/g;

/**
 * Converts a string from snake_case to camelCase
 **/
export function toCamelCase(str) {
  return str.replace(re, function (match) {
    return match.substring(1).charAt(0).toUpperCase() + match.substring(2);
  });
}

/**
 * Provides a human view on time.
 * For example:
 *
 * baseObject.humanized.time.fieldName will produce a string representation of fieldName
 * if fieldName is a date.
 *
 * Can be attached to any object as a mixin.
 *
 * baseObject.humanized = new Humanizer(baseObject)
 **/
export class Humanizer {
  constructor(base) {
    this._time = new Proxy(base, {
      get(target, prop, receiver) {
        let value = Reflect.get(target, prop, receiver);
        if (typeof value === "undefined") {
          return value;
        }
        if (typeof value === "string") {
          value = new Date(value);
        }
        // Hack to get a string representation of the timezone.
        // Borrowed from the old code and probably the best approach without using
        // an external library...
        const str = value.toString();
        const timezone = str.substring(str.indexOf("("));
        return `${value.toLocaleString()} ${timezone}`;
      },
    });
  }

  get time() {
    return this._time;
  }
}

/**
 * Adds `baseObject.humanize.time.timeField` using the humanizer mixin and converts all time columns to dates.
 **/
export function sanitizeAndHumanizeTime(obj, timeFields) {
  // Gracefully handle an undefined object to be safe.
  if (typeof obj === "undefined") {
    return;
  }
  for (const field of timeFields) {
    const value = obj[field];
    if (typeof value === "string") {
      obj[field] = new Date(value);
    }
  }
  obj.humanized = new Humanizer(obj);
}

export function humanize(obj) {
  if (typeof obj === "undefined") {
    return;
  }
  obj.humanized = new Humanizer(obj);
  return obj;
}
