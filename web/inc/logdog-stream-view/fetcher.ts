/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use)) of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

///<reference path="../logdog-stream/logdog.ts" />
///<reference path="../logdog-stream/client.ts" />
///<reference path="../luci-operation/operation.ts" />

namespace LogDog {

  /** Options that can be passed to fetch operations. */
  export type FetcherOptions = {
    /**
     * The maximum number of bytes to fetch. If undefined, no maximum will be
     * specified, and the service will constrain the results.
     */
    byteCount?: number;
    /**
     * The maximum number of logs to fetch. If undefined, no maximum will be
     * specified, and the service will constrain the results.
     */
    logCount?: number;
    /** If defined and true, allow a fetch to return non-continuous entries. */
    sparse?: boolean;
  };

  /** The Fetcher's current status. */
  export enum FetchStatus {
    // Not doing anything.
    IDLE,
    // Attempting to load log data.
    LOADING,
    // We're waiting for the log stream to emit more logs.
    STREAMING,
    // The log stream is missing.
    MISSING,
    // The log stream encountered an error.
    ERROR,
    // The operaiton has been cancelled.
    CANCELLED,
  }

  /** Fetch represents a single fetch operation. */
  export class Fetch {
    readonly op: luci.Operation;
    private stateChangedCallbacks = new Array<(f: Fetch) => void>();

    constructor(
        private ctx: FetchContext, readonly p: Promise<LogDog.LogEntry[]>) {
      this.op = this.ctx.op;

      // We will now get notifications if our Context's state changes.
      this.ctx.stateChangedCallback = this.onStateChanged.bind(this);
    }

    get lastStatus(): FetchStatus {
      return this.ctx.lastStatus;
    }

    get lastError(): Error|undefined {
      return this.ctx.lastError;
    }

    addStateChangedCallback(cb: (f: Fetch) => void) {
      this.stateChangedCallbacks.push(cb);
    }

    private onStateChanged() {
      this.stateChangedCallbacks.forEach(cb => cb(this));
    }
  }

  /**
   * FetchContext is an internal context used to pair all of the common
   * parameters involved in a fetch operation.
   */
  class FetchContext {
    private _lastStatus = FetchStatus.IDLE;
    private _lastError?: Error;

    stateChangedCallback: () => void;

    constructor(readonly op: luci.Operation) {
      // If our operation is cancelled, update our status to note this.
      op.addCancelCallback(() => {
        this._lastStatus = FetchStatus.CANCELLED;
        this._lastError = undefined;
      });
    }

    get lastStatus(): FetchStatus {
      return this._lastStatus;
    }

    get lastError(): Error|undefined {
      return this._lastError;
    }

    updateStatus(st: FetchStatus, err?: Error) {
      if (this.op.cancelled) {
        // No more status updates, force cancelled.
        st = FetchStatus.CANCELLED;
        err = undefined;
      }

      if (st === this._lastStatus && err === this._lastError) {
        return;
      }

      this._lastStatus = st;
      this._lastError = err;
      this.notifyStateChanged();
    }

    notifyStateChanged() {
      // If our Fetch has assigned our callback, notify it.
      if (this.stateChangedCallback) {
        this.stateChangedCallback();
      }
    }
  }

  /**
   * Fetcher is responsible for fetching LogDog log stream entries from the
   * remote service via an RPC client.
   *
   * Fetcher is responsible for wrapping the raw RPC calls and their results,
   * and retrying calls due to:
   *
   * - Transient failures (via RPC client).
   * - Missing stream (assumption is that the stream is still being ingested and
   *   registered, and therefore a repeated retry is appropriate).
   * - Streaming stream (log stream is not terminated, but more records are not
   *   yet available).
   *
   * The interface that Fetcher presents to its caller is a simple Promise-based
   * method to retrieve log stream data.
   *
   * Fetcher offers fetching via "get", "getAll", and "getLatest".
   */
  export class Fetcher {
    private debug = false;
    private static maxLogsPerGet = 0;

    private lastDesc: LogDog.LogStreamDescriptor;
    private lastState: LogDog.LogStreamState;

    private static missingRetry: luci.Retry = {delay: 5000, maxDelay: 15000};
    private static streamingRetry: luci.Retry = {delay: 1000, maxDelay: 5000};

    constructor(
        private client: LogDog.Client, readonly stream: LogDog.StreamPath) {}

    get desc() {
      return this.lastDesc;
    }
    get state() {
      return this.lastState;
    }

    /**
     * Returns the log stream's terminal index.
     *
     * If no terminal index is known (the log is still streaming) this will
     * return -1.
     */
    get terminalIndex(): number {
      return ((this.lastState) ? this.lastState.terminalIndex : -1);
    }

    /** Archived returns true if this log stream is known to be archived. */
    get archived(): boolean {
      return (!!(this.lastState && this.lastState.archive));
    }

    /**
     * Returns a Promise that will resolve to the next block of logs in the
     * stream.
     *
     * @return {Promise[LogDog.LogEntry[]]} A Promise that will resolve to the
     *     next block of logs in the stream.
     */
    get(op: luci.Operation, index: number, opts: FetcherOptions): Fetch {
      let ctx = new FetchContext(op);
      return new Fetch(ctx, this.getIndex(ctx, index, opts));
    }

    /**
     * Returns a Promise that will resolve to "count" log entries starting at
     * "startIndex".
     *
     * If multiple RPC calls are required to retrieve "count" entries, these
     * will be scheduled, and the Promise will block until the full set of
     * requested stream entries is retrieved.
     */
    getAll(op: luci.Operation, startIndex: number, count: number): Fetch {
      // Request the tail walkback logs. Since our request for N logs may return
      // <N logs, we will repeat the request until all requested logs have been
      // obtained.
      let allLogs: LogDog.LogEntry[] = [];

      let ctx = new FetchContext(op);
      let getIter = (): Promise<LogDog.LogEntry[]> => {
        op.assert();

        if (count <= 0) {
          return Promise.resolve(allLogs);
        }

        // Perform Gets until we have the requested number of logs. We don't
        // have to constrain the "logCount" parameter b/c we automatically do
        // that in getIndex.
        let opts: FetcherOptions = {
          logCount: count,
          sparse: true,
        };
        return this.getIndex(ctx, startIndex, opts).then(logs => {
          op.assert();

          if (logs && logs.length) {
            allLogs.push.apply(allLogs, logs);
            startIndex += logs.length;
            count -= logs.length;
          }
          if (count > 0) {
            // Recurse.
          }
          return Promise.resolve(allLogs);
        });
      };
      return new Fetch(ctx, getIter());
    }

    /**
     * Fetches the latest log entry.
     */
    getLatest(op: luci.Operation): Fetch {
      let errNoLogs = new Error('no logs, streaming');
      let streamingRetry = new luci.RetryIterator(Fetcher.streamingRetry);
      let ctx = new FetchContext(op);
      return new Fetch(
          ctx,
          streamingRetry.do(
              () => {
                return this.doTail(ctx).then(logs => {
                  if (!(logs && logs.length)) {
                    throw errNoLogs;
                  }
                  return logs;
                });
              },
              (err: Error, delay: number) => {
                if (err !== errNoLogs) {
                  throw err;
                }

                // No logs were returned, and we expect logs, so we're
                // streaming. Try again after a delay.
                ctx.updateStatus(FetchStatus.STREAMING);
                console.warn(
                    this.stream,
                    `: No logs returned; retrying after ${delay}ms...`);
              }));
    }

    private getIndex(ctx: FetchContext, index: number, opts: FetcherOptions):
        Promise<LogDog.LogEntry[]> {
      // (Testing) Constrain our max logs, if set.
      if (Fetcher.maxLogsPerGet > 0) {
        if ((!opts.logCount) || opts.logCount > Fetcher.maxLogsPerGet) {
          opts.logCount = Fetcher.maxLogsPerGet;
        }
      }

      // We will retry continuously until we get a log (streaming).
      let streamingRetry = new luci.RetryIterator(Fetcher.streamingRetry);
      let errNoLogs = new Error('no logs, streaming');
      return streamingRetry
          .do(
              () => {
                // If we're asking for a log beyond our stream, don't bother.
                if (this.terminalIndex >= 0 && index > this.terminalIndex) {
                  return Promise.resolve([]);
                }

                return this.doGet(ctx, index, opts).then(logs => {
                  ctx.op.assert();

                  if (!(logs && logs.length)) {
                    // (Retry)
                    throw errNoLogs;
                  }

                  return logs;
                });
              },
              (err: Error, delay: number) => {
                ctx.op.assert();

                if (err !== errNoLogs) {
                  throw err;
                }

                // No logs were returned, and we expect logs, so we're
                // streaming. Try again after a delay.
                ctx.updateStatus(FetchStatus.STREAMING);
                console.warn(
                    this.stream,
                    `: No logs returned; retrying after ${delay}ms...`);
              })
          .then(logs => {
            ctx.op.assert();

            // Since we allow non-contiguous Get, we may get back more logs than
            // we actually expected. Prune any such additional.
            if (opts.sparse && opts.logCount && opts.logCount > 0) {
              let maxStreamIndex = index + opts.logCount - 1;
              logs = logs.filter(le => le.streamIndex <= maxStreamIndex);
            }
            return logs;
          });
    }

    private doGet(ctx: FetchContext, index: number, opts: FetcherOptions):
        Promise<LogDog.LogEntry[]> {
      let request: LogDog.GetRequest = {
        project: this.stream.project,
        path: this.stream.path,
        state: (this.terminalIndex < 0),
        index: index,
      };
      if (opts.sparse || this.archived) {
        // This log stream is archived. We will relax the contiguous requirement
        // so we can render sparse log streams.
        request.nonContiguous = true;
      }
      if (opts.byteCount && opts.byteCount > 0) {
        request.byteCount = opts.byteCount;
      }
      if (opts.logCount && opts.logCount > 0) {
        request.logCount = opts.logCount;
      }

      if (this.debug) {
        console.log('logdog.Logs.Get:', request);
      }

      // Perform our Get, waiting until the stream actually exists.
      let missingRetry = new luci.RetryIterator(Fetcher.missingRetry);
      return missingRetry
          .do(
              () => {
                ctx.updateStatus(FetchStatus.LOADING);
                return this.client.get(request);
              },
              this.doRetryIfMissing(ctx))
          .then((resp: GetResponse) => {
            let fr = FetchResult.make(resp, this.lastDesc);
            return this.afterProcessResult(ctx, fr);
          });
    }

    private doTail(ctx: FetchContext): Promise<LogDog.LogEntry[]> {
      let missingRetry = new luci.RetryIterator(Fetcher.missingRetry);
      return missingRetry
          .do(
              () => {
                ctx.updateStatus(FetchStatus.LOADING);
                let needsState = (this.terminalIndex < 0);
                return this.client.tail(this.stream, needsState);
              },
              this.doRetryIfMissing(ctx))
          .then((resp: GetResponse) => {
            let fr = FetchResult.make(resp, this.lastDesc);
            return this.afterProcessResult(ctx, fr);
          });
    }

    private afterProcessResult(ctx: FetchContext, fr: FetchResult):
        LogDog.LogEntry[] {
      if (this.debug) {
        if (fr.logs.length) {
          console.log(
              'Request returned:', fr.logs[0].streamIndex, '..',
              fr.logs[fr.logs.length - 1].streamIndex, fr.desc, fr.state);
        } else {
          console.log('Request returned no logs:', fr.desc, fr.state);
        }
      }

      ctx.updateStatus(FetchStatus.IDLE);
      if (fr.desc) {
        this.lastDesc = fr.desc;
      }
      if (fr.state) {
        this.lastState = fr.state;
      }
      return fr.logs;
    }

    private doRetryIfMissing(ctx: FetchContext) {
      return (err: Error, delay: number) => {
        ctx.op.assert();

        // Is this a gRPC Error?
        let grpc = luci.GrpcError.convert(err);
        if (grpc && grpc.code === luci.Code.NOT_FOUND) {
          ctx.updateStatus(FetchStatus.MISSING);

          console.warn(
              this.stream, ': Is not found:', err,
              `; retrying after ${delay}ms...`);
          return;
        }

        ctx.updateStatus(FetchStatus.ERROR, err);
        throw err;
      };
    }
  }

  /**
   * The result of a log stream fetch, for internal usage.
   *
   * It will include zero or more log entries, and optionally (if requested)
   * the log stream's descriptor and state.
   */
  class FetchResult {
    constructor(
        readonly logs: LogDog.LogEntry[],
        readonly desc?: LogDog.LogStreamDescriptor,
        readonly state?: LogDog.LogStreamState) {}

    static make(resp: GetResponse, desc: LogDog.LogStreamDescriptor):
        FetchResult {
      let loadDesc: LogDog.LogStreamDescriptor|undefined;
      if (resp.desc) {
        desc = loadDesc = LogDog.LogStreamDescriptor.make(resp.desc);
      }

      let loadState: LogDog.LogStreamState|undefined;
      if (resp.state) {
        loadState = LogDog.LogStreamState.make(resp.state);
      }

      let logs = (resp.logs || []).map(le => LogDog.LogEntry.make(le, desc));
      return new FetchResult(logs, loadDesc, loadState);
    }
  }
}
