/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

///<reference path="../logdog-stream/logdog.ts" />
///<reference path="../logdog-stream/client.ts" />

namespace LogDog {

  /** Returns true if "s" has glob characters in it. */
  export function isQuery(s: string): boolean {
    return (s.indexOf('*') >= 0);
  }

  /**
   * Issues a query and iteratively pulls up to limit results.
   */
  export function queryAll(
      client: LogDog.Client, req: QueryRequest,
      limit: number): Promise<QueryResult[]> {
    let results: QueryResult[] = [];
    let cursor: string;
    limit = (limit || 100);

    let fetchRound = (first: boolean): Promise<QueryResult[]> => {
      let remaining = (limit - results.length);
      if (remaining <= 0 || (!(first || cursor))) {
        return Promise.resolve(results);
      }

      return client.query(req, cursor, remaining).then(round => {
        if (round[0]) {
          results.push.apply(results, round[0]);
        }
        cursor = round[1];
        return fetchRound(false);
      });
    };

    return fetchRound(true);
  }
}
