/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

///<reference path="logdog.ts" />
///<reference path="../rpc/client.ts" />

namespace LogDog {

  export type GetRequest = {
    project: string; path: string; state: boolean; index: number;

    nonContiguous?: boolean;
    byteCount?: number;
    logCount?: number;
  };

  export type TailRequest = {project: string; path: string; state: boolean;};

  // Type of a "Get" or "Tail" response (protobuf).
  export type GetResponse = {state: any; desc: any; logs: any[];};

  /** Configurable set of log stream query parameters. */
  export type QueryRequest = {
    project: string; getMeta?: boolean; path?: string; contentType?: string;
    streamType?: LogDog.StreamType;
    purged?: boolean;
    newer?: Date;
    older?: Date;
    protoVersion?: string;
    tags?: {[key: string]: string};
  };

  /** The result of a log stream query. */
  export type QueryResult = {
    stream: LogDog.StreamPath; state?: LogDog.LogStreamState;
    desc?: LogDog.LogStreamDescriptor;
  };

  export class Client {
    debug = false;

    constructor(private client: luci.Client) {}

    /** Get executes a Get RPC. */
    async get(req: GetRequest): Promise<GetResponse> {
      if (this.debug) {
        console.log('logdog.Logs.Get:', req);
      }
      return this.client.call('logdog.Logs', 'Get', req);
    }

    /** Tail executes a Tail RPC. */
    async tail(stream: StreamPath, state: boolean): Promise<GetResponse> {
      let request: TailRequest = {
        project: stream.project,
        path: stream.path,
        state: state,
      };

      if (this.debug) {
        console.log('logdog.Logs.Tail:', request);
      }
      return this.client.call('logdog.Logs', 'Tail', request);
    }

    /**
     * Query executes a Query RPC.
     *
     * @param params The query request parameters.
     * @param cursor The cursor to supply. Can be empty for no cursor.
     * @param limit The maximum number of query results to return.
     *
     * @return a Promise that resolves to the query results and continuation
     *     cursor. The cursor may be empty if the query finished.
     */
    async query(params: QueryRequest, cursor = '', limit = 0):
        Promise<[QueryResult[], string]> {
      let project = params.project;
      let body: any = {
        project: project,
        path: params.path,
        content_type: params.contentType,
        proto_version: params.protoVersion,
        tags: params.tags,

        next: cursor,
        max_results: limit,
      };

      let trinary = (v: boolean): string => {
        return ((v) ? 'YES' : 'NO');
      };
      if (params.purged != null) {
        body.purged = trinary(params.purged);
      }

      if (params.streamType !== undefined) {
        body.stream_type = {value: LogDog.StreamType[params.streamType]};
      }
      if (params.newer) {
        body.newer = params.newer.toISOString();
      }
      if (params.older) {
        body.older = params.older.toISOString();
      }

      type responseType = {
        streams: {
          path: string,
          state: any,
          desc: any,
        }[];
        next: string;
      };
      let resp: responseType =
          await this.client.call('logdog.Logs', 'Query', body);

      // Package the response in QueryResults.
      let results = (resp.streams || []).map(entry => {
        let res: QueryResult = {
          stream: new LogDog.StreamPath(project, entry.path),
        };
        if (entry.state) {
          res.state = LogDog.LogStreamState.make(entry.state);
        }
        if (entry.desc) {
          res.desc = LogDog.LogStreamDescriptor.make(entry.desc);
        }
        return res;
      });
      return [results, resp.next];
    }
  }
}
