// Copyright 2019 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

export const ONGOING_STATES = new Set(["PENDING", "RUNNING"]);

export const EXCEPTIONAL_STATES = new Set([
  "TIMED_OUT",
  "EXPIRED",
  "NO_RESOURCE",
  "CANCELED",
  "KILLED",
]);

export const COUNT_FILTERS = [
  { label: "Total", value: "...", filter: "QUERY_ALL" },
  { label: "Success", value: "...", filter: "QUERY_COMPLETED_SUCCESS" },
  { label: "Failure", value: "...", filter: "QUERY_COMPLETED_FAILURE" },
  { label: "Pending", value: "...", filter: "QUERY_PENDING" },
  { label: "Running", value: "...", filter: "QUERY_RUNNING" },
  { label: "Timed Out", value: "...", filter: "QUERY_TIMED_OUT" },
  { label: "Bot Died", value: "...", filter: "QUERY_BOT_DIED" },
  { label: "Client Error", value: "...", filter: "QUERY_CLIENT_ERROR" },
  { label: "Deduplicated", value: "...", filter: "QUERY_DEDUPED" },
  { label: "Expired", value: "...", filter: "QUERY_EXPIRED" },
  { label: "No Resource", value: "...", filter: "QUERY_NO_RESOURCE" },
  { label: "Canceled", value: "...", filter: "QUERY_CANCELED" },
  { label: "Killed", value: "...", filter: "QUERY_KILLED" },
];

export const FILTER_STATES = [
  "ALL",
  "COMPLETED",
  "COMPLETED_SUCCESS",
  "COMPLETED_FAILURE",
  "RUNNING",
  "PENDING",
  "PENDING_RUNNING",
  "BOT_DIED",
  "DEDUPED",
  "TIMED_OUT",
  "EXPIRED",
  "NO_RESOURCE",
  "CANCELED",
  "KILLED",
  "CLIENT_ERROR",
];
