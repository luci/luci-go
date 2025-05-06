// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import * as human from "common-sk/modules/human";

import { EXCEPTIONAL_STATES, ONGOING_STATES } from "../task";
import {
  humanDuration,
  humanize,
  sanitizeAndHumanizeTime,
  timeDiffExact,
} from "../util";

/** canRetry returns if the given task can be retried.
 *  See https://crbug.com/936530 for one case in which it should not.
 */
export function canRetry(request) {
  return request && request.properties && request.properties.idempotent;
}

/** cipdLink constructs a URL to a CIPD resource given a version string
 *  and a CIPD server URL.
 */
export function cipdLink(actualVersion, server) {
  // actualVersion looks like infra/python/cpython/windows-amd64:1ba7...
  if (!actualVersion || !server) {
    return undefined;
  }
  const splits = actualVersion.split(":");
  if (splits.length !== 2) {
    return undefined;
  }
  const pkg = splits[0];
  const version = splits[1];
  return `${server}/p/${pkg}/+/${version}`;
}

/** durationChart returns an array of times to be displayed in a chart.
 *  These times are pending time, overhead before, task duration, and
 *  overhead after. We truncate them to 1 decimal place for display.
 */
export function durationChart(result) {
  const oneDecimalPlace = function (a) {
    if (!a) {
      return 0.0;
    }
    return Math.round(a * 10) / 10;
  };
  let preOverhead = 0;
  let postOverhead = 0;
  // These are only put in upon task completion.
  if (result.performanceStats) {
    postOverhead =
      (result.performanceStats.isolatedUpload &&
        result.performanceStats.isolatedUpload.duration) ||
      0;
    // We only know the certain timings of isolating. To get
    // close enough (tm) overhead timings, we assume CIPD is the only
    // other source of overhead and all of CIPD's overhead is done pre-task.
    preOverhead = result.performanceStats.botOverhead - postOverhead;
  }
  return [result.pending, preOverhead, result.duration, postOverhead].map(
    oneDecimalPlace
  );
}

/** firstDimension returns the first entry in an array of dimensions for
 *  a given key or null if the dimension array is malformed or key is not found.
 */
export function firstDimension(dimensionArr, key) {
  const dimensions = dimensionArr.filter(function (d) {
    return d.key === key;
  });
  if (!dimensions.length) {
    return null;
  }
  const values = dimensions[0].value;
  if (!values.length) {
    return null;
  }
  return values[0];
}

/** hasRichOutput returns true if a task supports a rich logs representation
 *  (e.g. Milo), false otherwise.
 */
export function hasRichOutput(ele) {
  if (!ele || !ele._request || !ele._request.tagMap) {
    return false;
  }
  const tagMap = ele._request.tagMap;
  return tagMap["allow_milo"] || tagMap["luci_project"];
}

/** humanState returns a human readable string corresponding to
 *  the task's state. It takes into account what slice this is
 *  and what slice ran, so as not to confuse the user.
 */
export function humanState(result, currentSliceIdx) {
  if (!result || !result.state) {
    return "";
  }
  if (
    currentSliceIdx !== undefined &&
    result.currentTaskSlice !== currentSliceIdx
  ) {
    return "THIS SLICE DID NOT RUN. Select another slice above.";
  }
  const state = result.state;
  if (state === "COMPLETED") {
    if (result.failure) {
      return "COMPLETED (FAILURE)";
    }
    if (wasDeduped(result)) {
      return "COMPLETED (DEDUPED)";
    }
    return "COMPLETED (SUCCESS)";
  }
  return state;
}

/** casLink constructs a URL to a CAS root directory given a CAS reference.
 */
export function casLink(host, ref) {
  return (
    `${host}/${ref.casInstance}/blobs/` +
    `${ref.digest.hash}/${ref.digest.sizeBytes || 0}/tree`
  );
}

/** isSummaryTask returns true if this task is a summary taskID
 *  and false otherwise.  See taskPageLink for more details.
 */
export function isSummaryTask(id) {
  return id && id.endsWith(0);
}

/** parseRequest pre-processes any data in the task request object.
 */
export function parseRequest(request) {
  if (!request) {
    return humanize({});
  }
  sanitizeAndHumanizeTime(request, TASK_TIMES);
  request.tagMap = {};
  request.tags = request.tags || [];
  for (const tag of request.tags) {
    const split = tag.split(":", 1);
    const key = split[0];
    const rest = tag.substring(key.length + 1);
    request.tagMap[key] = rest;
  }
  return request;
}

/** parseRequest pre-processes any data in the task result object.
 */
export function parseResult(result) {
  if (!result) {
    return humanize({});
  }
  sanitizeAndHumanizeTime(result, TASK_TIMES);
  const now = new Date();
  // Running and bot_died tasks have no duration set, so we can figure it out.
  if (!result.duration && result.state === "RUNNING" && result.startedTs) {
    result.duration = (now - result.startedTs) / 1000;
  } else if (
    !result.duration &&
    result.state === "BOT_DIED" &&
    result.startedTs &&
    result.abandonedTs
  ) {
    result.duration = (result.abandonedTs - result.startedTs) / 1000;
  }
  // Make the duration human readable
  result.humanDuration = humanDuration(result.duration);
  if (result.state === "RUNNING") {
    result.humanDuration += "*";
  } else if (result.state === "BOT_DIED") {
    result.humanDuration += " -- died";
  }

  const end = result.startedTs || result.abandonedTs || new Date();
  if (!result.createdTs) {
    // This should never happen
    result.pending = 0;
    result.humanPending = "";
  } else if (end <= result.createdTs) {
    // In the case of deduplicated tasks, started_ts comes before the task.
    result.pending = 0;
    result.humanPending = "0s";
  } else {
    result.pending = (end - result.createdTs) / 1000; // convert to seconds.
    result.humanPending = timeDiffExact(result.createdTs, end);
  }
  result.currentTaskSlice = parseInt(result.currentTaskSlice) || 0;
  return result;
}

const TASK_ID_PLACEHOLDER = "${SWARMING_TASK_ID}";

/** richLogsLink returns a URL to a rich logs representation (e.g. Milo)
 *  given information in the request/server_details of ele. If the data
 *  is not there (e.g. the task doesn't support it), undefined will be returned.
 */
export function richLogsLink(ele) {
  if (!ele || !ele._request || !ele._request.tagMap) {
    return undefined;
  }
  const tagMap = ele._request.tagMap;
  const miloHost = tagMap["milo_host"];
  let logs = tagMap["log_location"];
  if (logs && miloHost) {
    logs = logs.replace("logdog://", "");
    if (logs.indexOf(TASK_ID_PLACEHOLDER) !== -1) {
      if (!ele._result || !ele._result.runId) {
        return undefined;
      }
      logs = logs.replace(TASK_ID_PLACEHOLDER, ele._result.runId);
    }
    return miloHost.replace("%s", logs);
  }
  const displayTemplate = ele.serverDetails.displayServerUrlTemplate;
  if (!displayTemplate || !ele._taskId) {
    return undefined;
  }
  return displayTemplate.replace("%s", ele._taskId);
}

/** sliceSchedulingDeadline returns a human readable time stamp of when a task
 *  slice expires.
 */
export function sliceSchedulingDeadline(slice, request) {
  if (!request.createdTs) {
    return "";
  }
  const delta = slice.expirationSecs * 1000;
  return human.localeTime(new Date(request.createdTs.getTime() + delta));
}

/** stateClass returns a class corresponding to the task's state.
 */
export function stateClass(result) {
  if (!result || !result.state) {
    return "";
  }
  const state = result.state;
  if (EXCEPTIONAL_STATES.has(state)) {
    return "exception";
  }
  if (state === "BOT_DIED") {
    return "bot_died";
  }
  if (state === "CLIENT_ERROR") {
    return "client_error";
  }
  if (ONGOING_STATES.has(state)) {
    return "pending_task";
  }
  if (state === "COMPLETED") {
    if (result.failure) {
      return "failed_task";
    }
  }
  return "";
}

/** taskCost returns a human readable cost in USD for a task.
 */
export function taskCost(result) {
  if (!result || !result.costsUsd || !result.costsUsd.length) {
    return 0;
  }
  return result.costsUsd[0].toFixed(4);
}

/** taskSchedulingDeadline returns a human readable time stamp of when a task
 *  expires, which is after any and all slices expire.
 */
export function taskSchedulingDeadline(request) {
  if (!request.createdTs) {
    return "";
  }
  const delta = request.expirationSecs * 1000;
  return human.localeTime(new Date(request.createdTs.getTime() + delta));
}

export function taskInfoClass(ele, result) {
  // Prevents a flash of grey while request and result load.
  if (!ele || !result || ele._currentSliceIdx === -1) {
    return "";
  }
  if (ele._currentSliceIdx !== result.currentTaskSlice) {
    return "inactive";
  }
  return "";
}

/** wasDeduped returns true or false depending on if this task was de-duped.
 */
export function wasDeduped(result) {
  return result.dedupedFrom;
}

/** wasPickedUp returns true iff a task was started.
 */
export function wasPickedUp(result) {
  return (
    result &&
    result.state !== "PENDING" &&
    result.state !== "NO_RESOURCE" &&
    result.state !== "CANCELED" &&
    result.state !== "EXPIRED"
  );
}

const TASK_TIMES = [
  "abandoned_ts",
  "bot_idle_since_ts",
  "completed_ts",
  "created_ts",
  "modified_ts",
  "started_ts",
  "abandonedTs",
  "botIdleSinceTs",
  "completedTs",
  "createdTs",
  "modifiedTs",
  "startedTs",
];

/** isFromBuildBucket returns true if a task comes from BuildBucket.
 */
export function isFromBuildBucket(request) {
  if (!request || !request.tags || !request.tags.length) {
    return false;
  }
  // BuildBucket adds specific tags to the Swarming task.
  // infra_superproject/+/0d182f6ece270468fafbd1d7b1a48b3d1b0e023e:infra/go/src/go.chromium.org/luci/buildbucket/appengine/tasks/swarming.go;l=659.
  const buildBucketTags = [
    "buildbucket_bucket",
    "buildbucket_build_id",
    "buildbucket_hostname",
  ];
  // Exits on the first BuildBucket tag found.
  return request.tags.some((tag) => {
    return buildBucketTags.some((bbTag) => {
      return tag.startsWith(bbTag);
    });
  });
}
