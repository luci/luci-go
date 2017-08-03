/*
  Copyright 2017 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

///<reference path="../logdog-stream/logdog.ts" />
///<reference path="../logdog-stream/client.ts" />
///<reference path="../luci-operation/operation.ts" />
///<reference path="../luci-sleep-promise/promise.ts" />
///<reference path="../rpc/client.ts" />

namespace LogDog {

  /** UI element used for rendering an individual query result. */
  type QueryEntry = {fullPath: string; title: string;};

  /**
   * Represents the fields of a "logdog-query-view" Polymer element used by
   * QueryView.
   */
  type QueryComponent = {
    $: {
      client: luci.PolymerClient;
      queryPanel: {project: string; path: string; streamType: string;},
    }

    _setSharedPrefix(prefix: string|null): void;
    _setQueryResults(results: QueryEntry[]): void;
  };

  const QUERY_LIMIT = 50;

  /** QueryView manages a "logdog-query-view" Polymer element. */
  export class QueryView {
    private currentQuery: luci.Operation|null;
    private client: LogDog.Client;

    constructor(readonly comp: QueryComponent) {}

    /**
     * Cancels any ongoing queries and clears all elements to return this to an
     * initializsed state.
     */
    reset() {
      this._cancelCurrentQuery();
      this.client = new LogDog.Client(new luci.Client(this.comp.$.client));

      this.comp._setSharedPrefix(null);
      this.comp._setQueryResults([]);
    }

    /**
     * Called to execute a new query.
     *
     * If a query is currently executing, it will be cancelled.
     *
     * When the query is complete, the UI will be updated with the results.
     */
    doQuery() {
      this._cancelCurrentQuery();

      let qp = this.comp.$.queryPanel;
      let project = qp.project;
      let params: LogDog.QueryRequest = {
        project: project,
        path: qp.path,
      };

      /**
       * Map the "Stream Type" text values in "logdog-query-panel" to their
       * streamType constants.
       */
      switch (qp.streamType) {
        case 'Text':
          params.streamType = LogDog.StreamType.TEXT;
          break;

        case 'Binary':
          params.streamType = LogDog.StreamType.BINARY;
          break;

        case 'Datagram':
          params.streamType = LogDog.StreamType.DATAGRAM;
          break;

        case 'Any':
        default:
          break;
      }

      let op = new luci.Operation();
      this.client.query(op, params, '', QUERY_LIMIT).then((result) => {
        // Do all of the results have the same prefix?
        let streams = result[0] || [];
        let sharedPrefix: string|null = null;
        if (streams.length > 0) {
          sharedPrefix = streams[0].stream.prefix;
          for (let i = 1; i < streams.length; i++) {
            if (streams[i].stream.prefix !== sharedPrefix) {
              sharedPrefix = null;
              break;
            }
          }
        }
        this.comp._setSharedPrefix(sharedPrefix);

        let queryResults = result[0].map((v): QueryEntry => {
          return {
            fullPath: v.stream.fullName(),
            title:
                ((sharedPrefix) ? ('.../+/' + v.stream.name) : v.stream.path),
          };
        });
        this.comp._setQueryResults(queryResults);
      });
      this.currentQuery = op;
    }

    private _cancelCurrentQuery() {
      if (this.currentQuery) {
        this.currentQuery.cancel();
        this.currentQuery = null;
      }
    }
  }
}
