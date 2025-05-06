// Copyright 2023 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

import { PrpcService } from "./common";

/**
 * Service to communicate with swarming.v2.Bots prpc service.
 */
export class BotsService extends PrpcService {
  get service() {
    return "swarming.v2.Bots";
  }

  /**
   * Calls the GetBot route.
   *
   *  @param {String} botId - identifier of the bot to retrieve.
   *
   *  @returns {Object} object with information about the bot in question.
   */
  bot(botId) {
    return this._call("GetBot", { botId: botId });
  }

  /**
   * Calls the ListBotTasks route
   *
   *  @param {String} botId - identifier of the bot to retrieve.
   *  @param {String} cursor - cursor retrieved from previous request to ListBotTasks.
   *
   *  @returns {Object} object containing both items and cursor fields. `items` contains a list of tasks associated with the Bot and `cursor` is a db cursor from the previous request.
   */
  tasks(botId, cursor) {
    const request = {
      sort: "QUERY_STARTED_TS",
      state: "QUERY_ALL",
      botId: botId,
      cursor: cursor,
      limit: 30,
      includePerformanceStats: true,
    };
    return this._call("ListBotTasks", request);
  }

  /**
   * Terminates a bot given a botId.
   *
   * @param {String} botId - identifier of bot to terminate.
   * @param {String} reason - user supplied reason to terminate the bot
   *
   * @returns {Object} with the shape {taskId: "some_task_id"} if the termination operation was initiated without error
   *
   **/
  terminate(botId, reason) {
    const request = {
      botId: botId,
      reason: reason,
    };
    return this._call("TerminateBot", request);
  }

  /**
   * Requests a list of BotEvents for a given botId
   *
   * @param {String} botId - identifier of bot from which to retrieve events.
   * @param {String} cursor - cursor from previous request.
   *
   * @returns {Object} with shape {cursor: "some_cursor", items: [... list of bot events ...]}
   **/
  events(botId, cursor) {
    const request = {
      limit: 50,
      botId: botId,
      cursor: cursor,
    };
    return this._call("ListBotEvents", request);
  }

  /**
   * Deletes bot with the given botId
   *
   * @param {String} botId - identifier of bot to delete.
   *
   * @returns {Object} with shape { deleted: bool } where deleted is true if the bot was actually deleted.
   **/
  delete(botId) {
    return this._call("DeleteBot", { botId: botId });
  }

  /**
   * Counts bots with given dimensions.
   *
   * @param {String} dimensions - object with shape [{key: string, value: string}]
   *
   * @returns {Object} BotsCount response object described here - https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#973
   **/
  count(dimensions) {
    return this._call("CountBots", { dimensions });
  }

  /**
   * Fetches bot dimensions which are present in a specific pool.
   * @param {String} pool is the pool to search in.
   *
   * @returns {Object} BotDimensionsResponse described here - https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#983
   **/
  dimensions(pool) {
    return this._call("GetBotDimensions", { pool });
  }

  /**
   * Fetches a list of bots matching given filters.
   *
   * @param {Object} request is a ListBotRequest described here - https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#1063
   *
   * @returns {Object} BotInfoListResponse described here - https://chromium.googlesource.com/infra/luci/luci-py/+/ba4f94742a3ce94c49432417fbbe3bf1ef9a1fa0/appengine/swarming/proto/api_v2/swarming.proto#965
   **/
  list(request) {
    return this._call("ListBots", request);
  }
}
