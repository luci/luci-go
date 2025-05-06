// Copyright 2023 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import { PrpcService } from "./common";

export class TasksService extends PrpcService {
  get service() {
    return "swarming.v2.Tasks";
  }

  /**
   * Cancels task for given taskId
   *
   * @param {string} taskId - id of task to cancel.
   * @param {boolean} killRunning - whether to kill task while running.
   *
   * @returns {Object} with shape {canceled, was_running} - see CancelResponse in https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#702
   **/
  cancel(taskId, killRunning) {
    return this._call("CancelTask", {
      taskId: taskId,
      killRunning: killRunning,
    });
  }

  /**
   * Gets the standard output of the task.
   *
   * @param {string} taskId - id of task to retrieve.
   * @param {number} offset - number of bytes from beginning of task output to start.
   * @param {number} length - number of bytes to retrieve.
   *
   * @returns {Object} TaskOutputResponse object described in https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#720
   **/
  stdout(taskId, offset, length) {
    return this._call("GetStdout", {
      taskId: taskId,
      offset: offset,
      length: length,
    });
  }

  /**
   * Retrieves task_request for given taskId.
   *
   * @param {string} taskId - id of task request to retrieve.
   *
   * @returns {Object} - TaskRequest object described in https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#618
   **/
  request(taskId) {
    return this._call("GetRequest", {
      taskId: taskId,
    });
  }

  /**
   * Retrieves task_result for givenTaskId.
   *
   * @param {string} taskId - id of task to retrieve
   * @param {boolean} includePerformanceStats - whether to include performance stats with the taskId
   *
   * @returns {Object} see - https://crsrc.org/i/luci/appengine/swarming/proto/api_v2/swarming.proto;l=738?q=TaskResultResponse&sq=
   **/
  result(taskId, includePerformanceStats) {
    return this._call("GetResult", {
      taskId: taskId,
      includePerformanceStats: includePerformanceStats,
    });
  }

  /**
   * Creates a new task with the given newTask specification.
   *
   * @param {Object} NewTask specification described in https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#526
   *
   * @returns {Object} returns a TaskRequestMetadataResponse described in https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#923
   **/
  new(newTaskSpec) {
    return this._call("NewTask", newTaskSpec);
  }

  /**
   * Counts tasks from a given query params.
   * If a state is not specified, QUERY_ALL will be used.
   *
   * @param {Object} filters must be a protojson representation of TasksCountRequest https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#1114
   *
   * @returns {Object} TasksCount object described in https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#917
   **/
  count(filters) {
    if (!filters.state) {
      filters = { ...filters, state: "QUERY_ALL" };
    }
    return this._call("CountTasks", filters);
  }

  /**
   * Fetches a list of tasks which conform a set of filters.
   * If a state is not specified, QUERY_ALL will be used.
   *
   * @param {Object} filters must be protojson representation of TasksRequest https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#1103
   *
   * @returns {Object} TaskListResponse defined in https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#903 . If cursor != "" then there are likely more results to retrieve.
   **/
  list(filters) {
    if (!filters.state) {
      filters = { ...filters, state: "QUERY_ALL" };
    }
    return this._call("ListTasks", filters);
  }

  /**
   * Mass cancels tasks which match specific filters.
   *
   * @param {Object} filters must be protojson representation of https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#654
   *
   * @returns {Object} response which displays https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#712
   **/
  massCancel(filters) {
    return this._call("CancelTasks", filters);
  }
}
