/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

import {luci_rpc} from "rpc/client";
import {LogDog} from "logdog-stream/logdog";

export enum StreamType {
  TEXT,
  BINARY,
  DATAGRAM,
}

export interface QueryParams {
  project: string;
  getMeta?: boolean;
  path?: string;
  contentType?: string;
  streamType?: StreamType;
  purged?: boolean;
  newer?: Date;
  older?: Date;
  protoVersion?: string;
  tags?: { [key:string]:string };
}

export class LogDogQuery {
  private client: luci_rpc.Client
  private params: QueryParams;

  constructor(client: luci_rpc.Client) {
    this.client = client;
  }

  /** Returns true if "s" has glob characters in it. */
  static isQuery(s: string): boolean {
    return (s.indexOf("*") >= 0);
  };

  get(params: QueryParams, cursor: string, limit: number):
    Promise<[QueryResult[], string]> {

    let project = params.project;
    let body: { [key:string]:any } = {
      project: project,
      path: params.path,
      content_type: params.contentType,
      proto_version: params.protoVersion,
      tags: params.tags,

      next: cursor,
      max_results: limit,
    };

    let trinary = (v: boolean): string => {
      return ((v) ? "YES" : "NO");
    }
    if ( params.purged !== null ) {
      body["purged"] = trinary(params.purged);
    }

    if ( params.streamType !== undefined ) {
      let filter: { value?: string } = {};
      switch (params.streamType) {
      case StreamType.TEXT:
        filter.value = "TEXT";
        break;
      case StreamType.BINARY:
        filter.value = "BINARY";
        break;
      case StreamType.DATAGRAM:
        filter.value = "DATAGRAM";
        break;
      }
      body["stream_type"] = filter;
    }
    if (params.newer) {
      body["newer"] = params.newer.toISOString();
    }
    if (params.older) {
      body["older"] = params.older.toISOString();
    }

    type responseType = {
      streams: {
        path: string,
        state: any,
        desc: any,
      }[];
      next: string;
    };

    return this.client.call("logdog.Logs", "Query", body).
      then( (resp: responseType): [QueryResult[], string] => {

        // Package the response in QueryResults.
        let results = (resp.streams || []).map( (entry): QueryResult => {
          return new QueryResult(
              new LogDog.Stream(project, entry.path),
              ((entry.state) ?
                  LogDog.makeLogStreamState(entry.state) : (null)),
              ((entry.desc) ?
                  LogDog.makeLogStreamDescriptor(entry.desc) : (null)),
          );
        });
        return [results, resp.next];
      });
  }

  /**
   * Issues a query and iteratively pulls up to "this.limit" results.
   */
  getAll(params: QueryParams, limit: number): Promise<QueryResult[]> {
    let results: QueryResult[] = [];
    let cursor: string = null;
    limit = (limit || 100);

    let fetchRound = (first: boolean): Promise<QueryResult[]> => {
      var remaining = (limit - results.length);
      if (remaining <= 0 || (! (first || cursor)) ) {
        return Promise.resolve(results);
      }

      return this.get(params, cursor, remaining).then( (round) => {
        if ( round[0] ) {
          results.push.apply(results, round[0]);
        }
        cursor = round[1];
        return fetchRound(false);
      });
    };

    return fetchRound(true);
  }
}

export class QueryResult {
  constructor(readonly stream: LogDog.Stream,
              readonly state?: LogDog.LogStreamState,
              readonly desc?: LogDog.LogStreamDescriptor) {}
}
