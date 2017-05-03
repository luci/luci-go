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
  export async function
  queryAll(client: LogDog.Client, req: QueryRequest, limit: number):
      Promise<QueryResult[]> {
    let results: QueryResult[] = [];
    let cursor = '';

    for (let remaining = (limit || 100); remaining > 0;) {
      console.log('Query', cursor, remaining);
      let resp = await client.query(req, cursor, remaining);
      if (!resp[0].length) {
        break;
      }

      results.push.apply(results, resp[0]);
      remaining -= resp[0].length;

      cursor = resp[1];
      if (!(cursor && cursor.length)) {
        break;
      }
    }

    return results;
  }
}
