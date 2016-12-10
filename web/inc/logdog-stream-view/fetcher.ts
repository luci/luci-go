/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

import {luci_rpc} from "rpc/client";
import * as luci_sleep_promise from "luci-sleep-promise/promise";
import {LogDog} from "logdog-stream/logdog";

export type FetcherOptions = {
  byteCount?: number;
  logCount?: number;
  sparse?: boolean;
}

// Type of a "Get" or "Tail" response (protobuf).
type GetResponse = {
  state: any;
  desc: any;
  logs: any[];
};

/** The Fetcher's current status. */
export enum FetcherStatus {
  // Not doing anything.
  IDLE,
  // Attempting to load log data.
  LOADING,
  // We're waiting for the log stream to emit more logs.
  STREAMING,
  // The log stream is missing.
  MISSING,
  // The log stream encountered an error.
  ERROR
}

export class Fetcher {
  private client: luci_rpc.Client;

  private debug: boolean = false;
  private static maxLogsPerGet = 0;

  private lastDesc: LogDog.LogStreamDescriptor;
  private lastState: LogDog.LogStreamState;
  private activePromise: Promise<LogDog.LogEntry[]>;

  private currentStatus: FetcherStatus = FetcherStatus.IDLE;
  private _lastError: Error;
  private statusChangedCallback: (() => void);

  private static missingRetry: luci_rpc.Retry = new luci_rpc.Retry(
    new luci_rpc.RetryIterator(null, 5000, 15000));

  private static streamingRetry = new luci_rpc.Retry(
    new luci_rpc.RetryIterator(null, 1000, 5000));

  constructor(client: luci_rpc.Client, readonly stream: LogDog.Stream) {
    this.client = client;
  }

  get desc() { return this.lastDesc; }
  get state() { return this.lastState; }

  get status() { return this.currentStatus; }

  /**
   * Returns the log stream's terminal index.
   *
   * If no terminal index is known, or if the log stream is still streaming,
   * this will return -1.
   */
  get terminalIndex(): number {
    return ( ( this.lastState ) ? this.lastState.terminalIndex : -1 );
  }

  /** Archived returns true if this log stream is known to be archived. */
  get archived(): boolean {
    return ( !! (this.lastState && this.lastState.archive) );
  }

  get lastError(): Error { return this._lastError; }

  private setCurrentStatus(st: FetcherStatus, err?: Error) {
    if ( st !== this.currentStatus || err !== this._lastError ) {
      this.currentStatus = st;
      this._lastError = err;

      if ( this.statusChangedCallback ) {
        this.statusChangedCallback();
      };
    }
  }

  setStatusChangedCallback(fn: () => void) {
    this.statusChangedCallback = fn;
  }

  /**
   * Returns a Promise that resolves to the next block of logs in the stream.
   *
   * @return {Promise[LogDog.LogEntry[]]} A Promise that will resolve to the
   *     next block of logs in the stream, or null if there are no logs to
   *     return.
   */
  get(index: number, opts?: FetcherOptions): Promise<LogDog.LogEntry[]> {
    return this.getIndex(index, opts);
  }

  getAll(startIndex: number, count: number): Promise<LogDog.LogEntry[]> {
    // Request the tail walkback logs. Since our request for N logs may return
    // <N logs, we will repeat the request until all requested logs have been
    // obtained.
    let allLogs = new Array<LogDog.LogEntry>();

    let getIter = (): Promise<LogDog.LogEntry[]> => {
      if ( count <= 0 ) {
        return Promise.resolve(allLogs);
      }

      // Perform Gets until we have the requested number of logs. We don't have
      // to constrain the "logCount" parameter b/c we automatically do that in
      // getIndex.
      let opts: FetcherOptions = {
        logCount: count,
        sparse: true,
      };
      return this.getIndex(startIndex, opts).then( (logs) => {
        if ( logs ) {
          allLogs.push.apply(allLogs, logs);
          startIndex += logs.length;
          count -= logs.length;
        }
        return getIter();
      });
    };
    return getIter();
  }

  tail(): Promise<LogDog.LogEntry[]> {
    let streamingRetry = Fetcher.streamingRetry.iterator();
    let tryTail = (): Promise<LogDog.LogEntry[]> => {
      return this.doTail().then( (logs): Promise<LogDog.LogEntry[]> => {
        if ( logs && logs.length ) {
          return Promise.resolve(logs);
        }

        // No logs were returned, and we expect logs, so we're streaming. Try
        // again after a delay.
        this.setCurrentStatus(FetcherStatus.STREAMING);
        let delay = streamingRetry.next();
        console.warn(this.stream,
            `: No logs returned; retrying after ${delay}ms...`);
        return luci_sleep_promise.sleep(delay).then( () => {
          return tryTail();
        });
      });
    };
    return tryTail();
  }


  private getIndex(index: number, opts: FetcherOptions):
    Promise<LogDog.LogEntry[]> {

    // (Testing) Constrain our max logs, if set.
    if ( Fetcher.maxLogsPerGet > 0 ) {
      if ( ! opts ) {
        opts = {};
      }
      if ( (!opts.logCount) || opts.logCount > Fetcher.maxLogsPerGet ) {
        opts.logCount = Fetcher.maxLogsPerGet;
      }
    }

    // We will retry continuously until we get a log (streaming).
    let streamingRetry = Fetcher.streamingRetry.iterator();
    let tryGet = (): Promise<LogDog.LogEntry[]> => {
      // If we're asking for a log beyond our stream, don't bother.
      if ( this.terminalIndex >= 0 && index > this.terminalIndex ) {
        return Promise.resolve(null);
      }

      return this.doGet(index, opts).
        then( (logs) => {
          if ( logs && logs.length ) {
            // Since we allow non-contiguous Get, we may get back more logs than
            // we actually expected. Prune any such additional.
            if ( opts.logCount > 0 ) {
              let maxStreamIndex = index + opts.logCount - 1;
              logs = logs.filter( (le) => {
                return le.streamIndex <= maxStreamIndex;
              } );
            }

            return Promise.resolve(logs);
          }

          // No logs were returned, and we expect logs, so we're streaming. Try
          // again after a delay.
          this.setCurrentStatus(FetcherStatus.STREAMING);
          let delay = streamingRetry.next();
          console.warn(this.stream,
              `: No logs returned; retrying after ${delay}ms...`);
          return luci_sleep_promise.sleep(delay).then( () => {
            return tryGet();
          });
        });
    };
    return tryGet();
  }

  private doGet(index: number, opts: FetcherOptions):
    Promise<LogDog.LogEntry[]> {

    let request: {
      project: string;
      path: string;
      state: boolean;
      index: number;

      nonContiguous?: boolean;
      byteCount?: number;
      logCount?: number;
    } = {
      project: this.stream.project,
      path: this.stream.path,
      state: (this.terminalIndex < 0),
      index: index,
    };
    if ( opts.sparse || this.archived ) {
      // This log stream is archived. We will relax the contiguous requirement
      // so we can render sparse log streams.
      request.nonContiguous = true;
    }
    if ( opts ) {
      if ( opts.byteCount > 0 ) {
        request.byteCount = opts.byteCount;
      }
      if ( opts.logCount > 0 ) {
        request.logCount = opts.logCount;
      }
    }

    if ( this.debug ) {
      console.log("logdog.Logs.Get:", request);
    }

    // Perform our Get, waiting until the stream actually exists.
    return this.doRetryIfMissing( (): Promise<FetchResult> => {
      return this.client.call("logdog.Logs", "Get", request).
          then( (resp: GetResponse): FetchResult => {
            return FetchResult.make(resp, this.lastDesc);
          });
    }).then( (fr) => {
      return this.afterProcessResult(fr);
    });
  }

  private doTail(): Promise<LogDog.LogEntry[]> {
    let request: {
      project: string;
      path: string;
      state: boolean;
    } = {
      project: this.stream.project,
      path: this.stream.path,
      state: (this.terminalIndex < 0),
    };

    if ( this.debug ) {
      console.log("logdog.Logs.Tail:", request);
    }

    return this.doRetryIfMissing( (): Promise<FetchResult> => {
      return this.client.call("logdog.Logs", "Tail", request).
          then( (resp: GetResponse): FetchResult => {
            return FetchResult.make(resp, this.lastDesc);
          });
    }).then( (fr) => {
      return this.afterProcessResult(fr);
    });
  }

  private afterProcessResult(fr: FetchResult): LogDog.LogEntry[] {
    if ( this.debug ) {
      if ( fr.logs.length ) {
        console.log("Request returned:", fr.logs[0].streamIndex, "..",
                    fr.logs[fr.logs.length-1].streamIndex, fr.desc, fr.state);
      } else {
        console.log("Request returned no logs:", fr.desc, fr.state);
      }
    }

    this.setCurrentStatus(FetcherStatus.IDLE);
    if ( fr.desc ) {
      this.lastDesc = fr.desc;
    }
    if ( fr.state ) {
      this.lastState = fr.state;
    }
    return fr.logs;
  }

  private doRetryIfMissing(fn: () => Promise<FetchResult>):
    Promise<FetchResult> {

    let missingRetry = Fetcher.missingRetry.iterator();

    let doIt = (): Promise<FetchResult> => {
      this.setCurrentStatus(FetcherStatus.LOADING);

      return fn().catch( (err: Error) => {
        // Is this a gRPC Error?
        let grpc = luci_rpc.GrpcError.convert(err);
        if ( grpc && grpc.code === luci_rpc.Code.NOT_FOUND ) {
          this.setCurrentStatus(FetcherStatus.MISSING);

          let delay = missingRetry.next();
          console.warn(this.stream, ": Is not found:", err,
                       `; retrying after ${delay}ms...`);
          return luci_sleep_promise.sleep(delay).then( () => {
            return doIt();
          });
        }

        this.setCurrentStatus(FetcherStatus.ERROR, err);
        throw err;
      });
    };
    return doIt();
  }
}

export class FetchResult {
  constructor(readonly logs: LogDog.LogEntry[],
              readonly desc?: LogDog.LogStreamDescriptor,
              readonly state?: LogDog.LogStreamState) {}

  static make(resp: GetResponse, desc: LogDog.LogStreamDescriptor):
    FetchResult {

    let loadDesc: LogDog.LogStreamDescriptor;
    if ( resp.desc ) {
      desc = loadDesc = LogDog.makeLogStreamDescriptor(resp.desc);
    }

    let loadState: LogDog.LogStreamState;
    if ( resp.state ) {
      loadState = LogDog.makeLogStreamState( resp.state );
    }

    let logs = (resp.logs || []).map( (le) => {
      return LogDog.makeLogEntry(le, desc);
    });
    return new FetchResult(logs, loadDesc, loadState);
  }

}
