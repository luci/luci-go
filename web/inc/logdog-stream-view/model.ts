/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

///<reference path="../logdog-stream/logdog.ts" />
///<reference path="../luci-operation/operation.ts" />
///<reference path="../luci-sleep-promise/promise.ts" />
///<reference path="../rpc/client.ts" />
///<reference path="fetcher.ts" />
///<reference path="query.ts" />
///<reference path="view.ts" />

namespace LogDog {

  /** Sentinel error: not authenticated. */
  let NOT_AUTHENTICATED = new Error('Not Authenticated');

  /**
   * Resolves an arbitrary error into special sentinels where appropriate.
   *
   * Currently supports resolving gRPC "unauthenticated" error into the
   * NOT_AUTHENTICATED sentinel.
   */
  function resolveErr(err: Error): Error {
    let grpc = luci.GrpcError.convert(err);
    if (grpc && grpc.code === luci.Code.UNAUTHENTICATED) {
      return NOT_AUTHENTICATED;
    }
    return err;
  }

  /** An individual log stream's status. */
  type LogStreamStatus = {
    stream: LogDog.StreamPath; state: string; fetchStatus: LogDog.FetchStatus;
    finished: boolean;
    needsAuth: boolean;
  };

  type StreamStatusCallback = (v: LogStreamStatus[]) => void;

  /** Location where a log stream should be loaded from. */
  export enum Location {
    /**
     * Represents the upper half of a split view. Logs start at 0 and go through
     * the HEAD point.
     */
    HEAD,
    /**
     * Represents the lower half of the split view. Logs start at the TAIL point
     * and go through the BOTTOM anchor point.
     */
    TAIL,
    /**
     * Represents an anchor point where the split occurred, obtained through a
     * single "Tail()" RPC call. If the terminal index is known when the split
     * occurs, this should be the terminal index.
     */
    BOTTOM,
  }

  /** A log loading profile type. */
  type ModelProfile = {
    /** The size of the first fetch. */
    initialFetchSize: number;
    /** The size of the first fetch, if loading multiple streams. */
    multiInitialFetchSize: number;
    /** The size of each subsequent fetch. */
    fetchSize: number;
  };

  /**
   * Model manages stream loading.
   *
   * Model pushes state update to the user interface with the ViewBinding that
   * is provided to it on construction.
   *
   * Model represents a view of the loading state of the configured log streams.
   * Log streams are individual named sets of consecutive records that are
   * either streaming (no known terminal index, so the expectation is that new
   * log records are being generated) or finished (known terminal index). The
   * Model's job is to understand the state of these streams, load their data,
   * and cause it to be output to the user interface.
   *
   * A Model is bound to a LogProvider, which manages a sequential series of log
   * records. If a single log stream is being fetched, the Model will use a
   * "LogStream" log provider. If multiple log streams are being fetched, the
   * Model will use the "AggregateLogStream" log provider, which internally
   * muxes log records from multiple "LogStream" providers together based on an
   * ordering.
   *
   * "AggregateLogStream" is an append-only LogProvider, meaning that it ONLY
   * supports emitting log records from its set of streams in order until no
   * more records are available.
   *
   * "LogStream" (single stream) offers a more complex set of options via its
   * "split" functionality. At the user's request, the LogStream can split,
   * jumping to the highest-indexed log entry in that stream. This is followed
   * up with three fetch options, which can be used interchangeably based on the
   * user's actions:
   * - HEAD fetches logs sequentially starting from 0 and ending at SPLIT.
   * - TAIL fetches logs backwards, starting from SPLIT and fetching towards 0.
   * - BOTTOM fetches logs from after SPLIT, fetching from SPLIT and ending at
   *   the log stream's terminal index.
   *
   * If the HEAD pointer ever reaches the TAIL pointer, all logs between 0 and
   * SPLIT have been filled in and the fetching situation turns back into a
   * standard sequential fetch.
   *
   * This scheme is designed around the capabilities of the LogDog log API.
   *
   * The view may optionally expose itself as "mobile", indicating to the Model
   * that it should use mobile-friendly tuning parameters.
   */
  export class Model {
    /** Default single-stream profile. */
    static DEFAULT_PROFILE: ModelProfile = {
      initialFetchSize: (1024 * 24),      // 24KiB
      multiInitialFetchSize: (1024 * 4),  // 4KiB
      fetchSize: (4 * 1024 * 1024),       // 4MiB
    };

    /** Profile to use for a mobile device, regardless of stream count. */
    static MOBILE_PROFILE: ModelProfile = {
      initialFetchSize: (1024 * 4),       // 4KiB
      multiInitialFetchSize: (1024 * 4),  // 4KiB
      fetchSize: (1024 * 256),            // 256KiB
    };

    /**
     * Amount of time after which we consider the build to have been loading
     * "for a while".
     */
    private static LOADING_WHILE_THRESHOLD_MS = 60000;  // 1 Minute

    /**
     * If >0, the maximum number of log lines to push at a time. We will sleep
     * in between these entries to allow the rest of the app to be responsive
     * during log dumping.
     */
    private static logAppendInterval = 4000;
    /**
     * Amount of time to sleep in between log append chunks. Larger numbers
     * will load logs slower, but will offer more opportunities for user
     * interaction in between log dumps.
     */
    private static logAppendDelay = 0;

    /** Our log provider. */
    private provider: LogProvider = this.nullProvider();

    /** The LogDog Coordinator client instance. */
    readonly client: LogDog.Client;

    /**
     * Promise that is resolved when authentication state changes. When this
     * happens, a new Promise is installed, and future authentication changes
     * will resolve the new Promise.
     */
    private authChangedPromise: Promise<void>;
    /**
     * Retained callback (Promise resolve) to invoke when authentication state
     * changes.
     */
    private authChangedCallback: (() => void);

    /** The current fetch Promise. */
    private currentOperation: luci.Operation|null = null;
    private currentFetchPromise: Promise<void>|null = null;

    /** Are we in automatic mode? */
    private isAutomatic = false;
    /** Are we tailing? */
    private fetchFromTail = false;
    /** Are we in the middle of rendering logs? */
    private rendering = false;

    private cachedLogStreamUrl: string|undefined = undefined;

    private loadingStateValue: LoadingState = LoadingState.NONE;
    private streamStatusValue: StreamStatusEntry[];

    /**
     * When rendering a Promise that will resolve when the render completes. We
     * use this to pipeline parallel data fetching and rendering.
     */
    private renderPromise: Promise<void>|null;

    constructor(
        client: luci.Client, readonly profile: ModelProfile,
        readonly view: ViewBinding) {
      this.client = new LogDog.Client(client);
      this.resetAuthChanged();
    }

    /**
     * Resolves user stream path strings to actual log streams.
     *
     * After the returned Promise resolves, log stream data can be fetched by
     * calling "fetch()".
     *
     * @return a Promise that will resolve once all log streams have been
     *   identified and the configured.
     */
    async resolve(paths: string[]) {
      this.reset();

      // For any path that is a query, execute that query.
      this.loadingState = LoadingState.RESOLVING;
      let streamBlocks: LogDog.StreamPath[][];
      try {
        streamBlocks = await this.resolvePathsIntoStreams(paths);
      } catch (err) {
        this.loadingState = LoadingState.ERROR;
        console.error('Failed to resolve log streams:', err);
        return;
      }

      // Flatten them all.
      let streams: LogDog.StreamPath[] = [];
      for (let streamBlock of streamBlocks) {
        streams.push.apply(streams, streamBlock);
      }

      let initialFetchSize = (streams.length <= 1) ?
          this.profile.initialFetchSize :
          this.profile.multiInitialFetchSize;

      // Generate a LogStream client entry for each composite stream.
      let logStreams = streams.map((stream) => {
        console.log('Resolved log stream:', stream);
        return new LogStream(
            this.client, stream, initialFetchSize, this.profile.fetchSize);
      });

      // Reset any existing state.
      this.reset();

      // If we have exactly one stream, then use it directly. This allows
      // it to split.
      let provider: LogProvider;
      switch (logStreams.length) {
        case 0:
          provider = this.nullProvider();
          break;
        case 1:
          provider = logStreams[0];
          break;
        default:
          provider = new AggregateLogStream(logStreams);
          break;
      }
      provider.setStreamStatusCallback((st: LogStreamStatus[]) => {
        if (this.provider === provider) {
          this.streamStatus = this.buildStreamStatus(st);
        }
      });
      this.provider = provider;
      this.setIdleLoadingState();
    }

    private setIdleLoadingState() {
      this.loadingState = (this.fetchedFullStream) ? (LoadingState.NONE) :
                                                     (LoadingState.PAUSED);
    }

    private resolvePathsIntoStreams(paths: string[]) {
      return Promise.all(paths.map(async path => {
        let stream = LogDog.StreamPath.splitProject(path);
        if (!LogDog.isQuery(stream.path)) {
          return [stream];
        }

        // This "path" is really a query. Construct and execute.
        //
        // If we failed due to an auth error, but our auth changed during the
        // operation, try again automatically.
        let query: LogDog.QueryRequest = {
          project: stream.project,
          path: stream.path,
          streamType: LogDog.StreamType.TEXT,
        };

        while (true) {
          let op = new luci.Operation();
          try {
            let results = await LogDog.queryAll(op, this.client, query, 100);
            return results.map(qr => qr.stream);
          } catch (err) {
            err = resolveErr(err);
            if (err !== NOT_AUTHENTICATED) {
              throw err;
            }

            await this.authChangedPromise;
          }
        }
      }));
    }

    /**
     * Resets the state.
     */
    reset() {
      this.view.clearLogEntries();
      this.clearCurrentOperation();
      this.provider = this.nullProvider();

      this.updateControls();
    }

    public get automatic() {
      return this.isAutomatic;
    }

    /**
     * Sets whether automatic loading is enabled.
     *
     * When enabled, a new fetch will immediately be dispatched when a previous
     * fetch finishes so long as there is still stream data to load.
     *
     * This isn't a setter because we need to be able to export it via
     * interface.
     */
    public set automatic(v: boolean) {
      this.isAutomatic = v && !this.fetchedFullStream;
      if (v) {
        // Passively kick off a new fetch.
        this.fetch(false);
      }
      this.updateControls();
    }

    private nullProvider(): LogProvider {
      return new AggregateLogStream([]);
    }

    /**
     * Clears the current operation, cancelling it if set. If the operation is
     * cleared, the current fetch and rendering states will be reset.
     *
     * @param op if provided, only cancel the current operation if it equals
     *     the supplied "op". If "op" does not match the current operation, it
     *     will be cancelled, but the current operation will be left in-tact.
     *     If "op" is undefined, cancel the current operation regardless.
     */
    clearCurrentOperation(op?: luci.Operation) {
      if (this.currentOperation) {
        if (op && op !== this.currentOperation) {
          // Conditional clear, and we are not the current operation, so do
          // nothing.
          op.cancel();
          return;
        }
        this.currentOperation.cancel();
        this.currentOperation = this.currentFetchPromise = null;
      }
    }

    private get loadingState(): LoadingState {
      return this.loadingStateValue;
    }
    private set loadingState(v: LoadingState) {
      if (v !== this.loadingStateValue) {
        this.loadingStateValue = v;
        this.updateControls();
      }
    }

    private get streamStatus(): StreamStatusEntry[] {
      return this.streamStatusValue;
    }
    private set streamStatus(st: StreamStatusEntry[]) {
      this.streamStatusValue = st;
      this.updateControls();
    }

    private updateControls() {
      let splitState: SplitState;
      if (this.providerCanSplit) {
        splitState = SplitState.CAN_SPLIT;
      } else {
        splitState =
            (this.isSplit) ? (SplitState.IS_SPLIT) : (SplitState.CANNOT_SPLIT);
      }

      this.view.updateControls({
        splitState: splitState,
        fullyLoaded: (this.fetchedFullStream && (!this.rendering)),
        logStreamUrl: this.logStreamUrl,
        loadingState: this.loadingState,
        streamStatus: this.streamStatus,
      });
    }

    /**
     * Note that the authentication state for the client has changed. This will
     * trigger an automatic fetch retry if our previous fetch failed due to
     * lack of authentication.
     */
    notifyAuthenticationChanged() {
      // Resolve our current "auth changed" Promise.
      this.authChangedCallback();
    }

    private resetAuthChanged() {
      // Resolve our previous function, if it's not already resolved.
      if (this.authChangedCallback) {
        this.authChangedCallback();
      }

      // Create a new Promise and install it.
      this.authChangedPromise = new Promise<void>((resolve, _) => {
        this.authChangedCallback = resolve;
      });
    }

    /**
     * Causes the view to immediately split, if possible, creating a region
     * representing the end of the log stream.
     *
     * If the view cannot split, or if it is already split, then this is a
     * no-op.
     *
     * This cancels the current fetch, if one is in progress.
     */
    async split() {
      // If we haven't already split, and our provider lets us split, then go
      // ahead and do so.
      if (this.providerCanSplit) {
        return this.fetchLocation(Location.TAIL, true);
      }
      return this.fetch(false);
    }

    /**
     * Fetches the next batch of logs.
     *
     * By default, if a fetch is already in-progress, this new fetch is ignored.
     * If cancel is true, then the current fetch should be cancelled and a new
     * fetch initiated.
     *
     * @param cancel true if we should abandon any current fetches.
     * @return A Promise that will resolve when the fetch operation is complete.
     */
    async fetch(cancel: boolean) {
      if (this.isSplit) {
        if (this.fetchFromTail) {
          // Next fetch grabs logs from the bottom (continue tailing).
          if (!this.fetchedEndOfStream) {
            return this.fetchLocation(Location.BOTTOM, cancel);
          } else {
            return this.fetchLocation(Location.TAIL, cancel);
          }
        }

        // We're split, but not tailing, so fetch logs from HEAD.
        return this.fetchLocation(Location.HEAD, cancel);
      }

      // We're not split. If we haven't reached end of stream, fetch logs from
      // HEAD.
      return this.fetchLocation(Location.HEAD, cancel);
    }

    /** Fetch logs from an explicit location. */
    async fetchLocation(l: Location, cancel: boolean): Promise<void> {
      if (this.currentFetchPromise && (!cancel)) {
        return this.currentFetchPromise;
      }
      this.clearCurrentOperation();

      // If our provider is finished, then do nothing.
      if (this.fetchedFullStream) {
        // There are no more logs.
        this.updateControls();
        return undefined;
      }

      // If we're asked to fetch BOTTOM, but we're not split, fetch HEAD
      // instead.
      if (l === Location.BOTTOM && !this.isSplit) {
        l = Location.HEAD;
      }

      // Rotate our fetch ID. This will effectively cancel any pending fetches.
      this.currentOperation = new luci.Operation();
      this.currentFetchPromise =
          this.fetchLocationImpl(l, this.currentOperation);
      return this.currentFetchPromise;
    }

    private async fetchLocationImpl(l: Location, op: luci.Operation) {
      for (let continueFetching = true; continueFetching;) {
        this.loadingState = LoadingState.LOADING;

        let loadingWhileTimer =
            new luci.Timer(Model.LOADING_WHILE_THRESHOLD_MS, () => {
              if (this.loadingState === LoadingState.LOADING) {
                this.loadingState = LoadingState.LOADING_BEEN_A_WHILE;
              }
            });

        let hasLogs = false;
        try {
          hasLogs = await this.fetchLocationRound(l, op);
        } catch (err) {
          console.log('Fetch failed with error:', err);

          // Cancel the timer here, since we may enter other states in this
          // "catch" block and we don't want to have the timer override them.
          loadingWhileTimer.cancel();

          // If we've been canceled, discard this result.
          if (err === luci.Operation.CANCELLED) {
            this.setIdleLoadingState();
            return;
          }

          if (err === NOT_AUTHENTICATED) {
            this.loadingState = LoadingState.NEEDS_AUTH;

            // We failed because we were not authenticated. Mark this
            // so we can retry if that state changes.
            await this.authChangedPromise;

            // Our authentication state changed during the fetch!
            // Retry automatically.
            continueFetching = true;
            continue;
          }

          console.error('Failed to load log streams:', err);
          return;
        } finally {
          loadingWhileTimer.cancel();
        }

        continueFetching = (this.automatic && hasLogs);
        if (continueFetching) {
          console.log('Automatic: starting next fetch.');
        }
      }

      if (this.renderPromise) {
        await this.renderPromise;
      }

      // Post-fetch cleanup.
      this.clearCurrentOperation(op);
      this.setIdleLoadingState();
      this.updateControls();
    }

    private async fetchLocationRound(l: Location, op: luci.Operation) {
      // Clear our loading state (updates controls automatically).
      let buf = await this.provider.fetch(op, l);
      let hadLogs = !!(buf.peek());

      // Resolve any previous rendering Promise that we have. This
      // makes sure our rendering and fetching don't get more than
      // one round out of sync.
      if (this.renderPromise) {
        await this.renderPromise;
      }

      // Initiate the next render. This will happen in the
      // background while we enqueue our next fetch.
      let doRender = async () => {
        this.rendering = true;
        await this.renderLogs(buf, l);
        this.rendering = false;
        this.updateControls();
      };
      this.renderPromise = doRender();

      if (this.fetchedFullStream) {
        return false;
      }
      return hadLogs;
    }

    private async renderLogs(buf: BufferedLogs, l: Location) {
      if (!(buf && buf.peek())) {
        return;
      }

      let logBlock: LogDog.LogEntry[] = [];

      // Create a promise loop to push logs at intervals.
      let lines = 0;
      for (let nextLog = buf.next(); (nextLog); nextLog = buf.next()) {
        // Add the next log to the append block.
        logBlock.push(nextLog);
        if (nextLog.text && nextLog.text.lines) {
          lines += nextLog.text.lines.length;
        }

        // Add logs until we reach our interval lines.
        // If we've exceeded our burst, then interleave a sleep (yield). This
        // will reduce user jank a bit.
        if (Model.logAppendInterval > 0 && lines >= Model.logAppendInterval) {
          this.appendBlock(logBlock, l);

          await luci.sleepPromise(Model.logAppendDelay);
          lines = 0;
        }
      }

      // If there are any buffered logs, append that block.
      this.appendBlock(logBlock, l);
    }

    /**
     * Appends the contents of the "block" array to the viewer, consuming
     * "block" in the process.
     *
     * Block will be reset (but not resized) to zero elements after appending.
     */
    private appendBlock(block: LogDog.LogEntry[], l: Location) {
      if (!block.length) {
        return;
      }

      console.log('Rendering', block.length, 'logs...');
      this.view.pushLogEntries(block, l);
      block.length = 0;

      // Update our status and controls.
      this.updateControls();
    }

    /**
     * Sets whether the next fetch will pull from the tail (end) or the top
     * (begining) region.
     *
     * This is only relevant when there is a log split.
     */
    setFetchFromTail(v: boolean) {
      this.fetchFromTail = v;
    }

    private buildStreamStatus(v: LogStreamStatus[]): StreamStatusEntry[] {
      let maxStatus = LogDog.FetchStatus.IDLE;
      let maxStatusCount = 0;
      let needsAuth = false;

      // Prune any finished entries and accumulate them for status bar change.
      v = (v || []).filter((st) => {
        needsAuth = (needsAuth || st.needsAuth);

        if (st.fetchStatus > maxStatus) {
          maxStatus = st.fetchStatus;
          maxStatusCount = 1;
        } else if (st.fetchStatus === maxStatus) {
          maxStatusCount++;
        }

        return (!st.finished);
      });

      return v.map((st): StreamStatusEntry => {
        return {
          name: '.../+/' + st.stream.name,
          desc: st.state,
        };
      });
    }

    private get providerCanSplit(): boolean {
      let split = this.provider.split();
      return (!!(split && split.canSplit()));
    }

    private get isSplit(): boolean {
      let split = this.provider.split();
      return (!!(split && split.isSplit()));
    }

    private get fetchedEndOfStream(): boolean {
      return (this.provider.fetchedEndOfStream());
    }

    private get fetchedFullStream(): boolean {
      return (this.fetchedEndOfStream && (!this.isSplit));
    }

    private get logStreamUrl(): string|undefined {
      if (!this.cachedLogStreamUrl) {
        this.cachedLogStreamUrl = this.provider.getLogStreamUrl();
      }
      return this.cachedLogStreamUrl;
    }
  }

  /** Generic interface for a log provider. */
  interface LogProvider {
    setStreamStatusCallback(cb: StreamStatusCallback): void;
    fetch(op: luci.Operation, l: Location): Promise<BufferedLogs>;
    getLogStreamUrl(): string|undefined;

    /** Will return null if this LogProvider doesn't support splitting. */
    split(): SplitLogProvider|null;
    fetchedEndOfStream(): boolean;
  }

  /** Additional methods for log stream splitting, if supported. */
  interface SplitLogProvider {
    canSplit(): boolean;
    isSplit(): boolean;
  }

  /** A LogStream is a LogProvider manages a single log stream. */
  class LogStream implements LogProvider {
    /**
     * Always begin with a small fetch. We'll disable this afterward the first
     * finishes.
     */
    private initialFetch = true;

    private fetcher: LogDog.Fetcher;
    private activeFetch: LogDog.Fetch|undefined;

    /** The log stream index of the next head() log. */
    private nextHeadIndex = 0;
    /**
     * The lowest log stream index of all of the tail logs. If this is <0, then
     * it is uninitialized.
     */
    private firstTailIndex = -1;
    /**
     * The next log stream index to fetch to continue pulling logs from the
     * bottom. If this is <0, it is uninitialized.
     */
    private nextBottomIndex = -1;

    private streamStatusCallback: StreamStatusCallback;

    /** The size of the tail walkback region. */
    private static TAIL_WALKBACK = 500;

    constructor(
        client: LogDog.Client, readonly stream: LogDog.StreamPath,
        readonly initialFetchSize: number, readonly fetchSize: number) {
      this.fetcher = new LogDog.Fetcher(client, stream);
    }

    private setActiveFetch(fetch: LogDog.Fetch): LogDog.Fetch {
      this.activeFetch = fetch;
      this.activeFetch.addStateChangedCallback((_: LogDog.Fetch) => {
        this.statusChanged();
      });
      return fetch;
    }

    get fetchStatus(): LogDog.FetchStatus {
      if (this.activeFetch) {
        return this.activeFetch.lastStatus;
      }
      return LogDog.FetchStatus.IDLE;
    }

    get fetchError(): Error|undefined {
      if (this.activeFetch) {
        return this.activeFetch.lastError;
      }
      return undefined;
    }

    async fetch(op: luci.Operation, l: Location) {
      // Determine which method to use based on the insertion point and current
      // log stream fetch state.
      let getLogs: Promise<LogDog.LogEntry[]>;
      switch (l) {
        case Location.HEAD:
          getLogs = this.getHead(op);
          break;

        case Location.TAIL:
          getLogs = this.getTail(op);
          break;

        case Location.BOTTOM:
          getLogs = this.getBottom(op);
          break;

        default:
          // Nothing to do.
          throw new Error('Unknown Location: ' + l);
      }

      try {
        let logs = await getLogs;
        this.initialFetch = false;
        this.statusChanged();
        return new BufferedLogs(logs);
      } catch (err) {
        throw resolveErr(err);
      }
    }

    get descriptor() {
      return this.fetcher.desc;
    }

    getLogStreamUrl(): string|undefined {
      let desc = this.descriptor;
      if (desc) {
        return (desc.tags || {})['logdog.viewer_url'];
      }
      return undefined;
    }

    setStreamStatusCallback(cb: StreamStatusCallback) {
      this.streamStatusCallback = cb;
    }

    private statusChanged() {
      if (this.streamStatusCallback) {
        this.streamStatusCallback([this.getStreamStatus()]);
      }
    }

    getStreamStatus(): LogStreamStatus {
      let pieces: string[] = [];
      let tidx = this.fetcher.terminalIndex;
      if (this.nextHeadIndex > 0) {
        pieces.push('1..' + this.nextHeadIndex);
      } else {
        pieces.push('0');
      }
      if (this.isSplit()) {
        if (tidx >= 0) {
          pieces.push('| ' + this.firstTailIndex + ' / ' + tidx);
          tidx = -1;
        } else {
          pieces.push(
              '| ' + this.firstTailIndex + '..' + this.nextBottomIndex +
              ' ...');
        }
      } else if (tidx >= 0) {
        pieces.push('/ ' + tidx);
      } else {
        pieces.push('...');
      }

      let needsAuth = false;
      let finished = this.finished;
      if (finished) {
        pieces.push('(Finished)');
      } else {
        switch (this.fetchStatus) {
          case LogDog.FetchStatus.IDLE:
          case LogDog.FetchStatus.LOADING:
            pieces.push('(Loading)');
            break;

          case LogDog.FetchStatus.STREAMING:
            pieces.push('(Streaming)');
            break;

          case LogDog.FetchStatus.MISSING:
            pieces.push('(Missing)');
            break;

          case LogDog.FetchStatus.ERROR:
            let err = this.fetchError;
            if (err) {
              err = resolveErr(err);
              if (err === NOT_AUTHENTICATED) {
                pieces.push('(Auth Error)');
                needsAuth = true;
              } else {
                pieces.push('(Error)');
              }
            } else {
              pieces.push('(Error)');
            }
            break;
          default:
            // Nothing to do.
            break;
        }
      }

      return {
        stream: this.stream,
        state: pieces.join(' '),
        finished: finished,
        fetchStatus: this.fetchStatus,
        needsAuth: needsAuth,
      };
    }

    split(): SplitLogProvider {
      return this;
    }

    isSplit(): boolean {
      // We're split if we have a bottom and we're not finished tailing.
      return (
          this.firstTailIndex >= 0 &&
          (this.nextHeadIndex < this.firstTailIndex));
    }

    canSplit(): boolean {
      return (!(this.isSplit() || this.caughtUp));
    }

    private get caughtUp(): boolean {
      // We're caught up if we have both a head and bottom index, and the head
      // is at or past the bottom.
      return (
          this.nextHeadIndex >= 0 && this.nextBottomIndex >= 0 &&
          this.nextHeadIndex >= this.nextBottomIndex);
    }

    fetchedEndOfStream(): boolean {
      let tidx = this.fetcher.terminalIndex;
      return (
          tidx >= 0 &&
          ((this.nextHeadIndex > tidx) || (this.nextBottomIndex > tidx)));
    }

    private get finished(): boolean {
      return ((!this.isSplit()) && this.fetchedEndOfStream());
    }

    private updateIndexes() {
      if (this.firstTailIndex >= 0) {
        if (this.nextBottomIndex < this.firstTailIndex) {
          this.nextBottomIndex = this.firstTailIndex + 1;
        }

        if (this.nextHeadIndex >= this.firstTailIndex &&
            this.nextBottomIndex >= 0) {
          // Synchronize our head and bottom pointers.
          this.nextHeadIndex = this.nextBottomIndex =
              Math.max(this.nextHeadIndex, this.nextBottomIndex);
        }
      }
    }

    private nextFetcherOptions(): LogDog.FetcherOptions {
      let opts: LogDog.FetcherOptions = {};
      if (this.initialFetch && this.initialFetchSize > 0) {
        opts.byteCount = this.initialFetchSize;
      } else if (this.fetchSize > 0) {
        opts.byteCount = this.fetchSize;
      }
      return opts;
    }

    private async getHead(op: luci.Operation) {
      this.updateIndexes();

      if (this.finished) {
        // Our HEAD region has met/surpassed our TAIL region, so there are no
        // HEAD logs to return. Only bottom.
        return [];
      }

      // If we have a tail pointer, only fetch HEAD up to that point.
      let opts = this.nextFetcherOptions();
      if (this.firstTailIndex >= 0) {
        opts.logCount = (this.firstTailIndex - this.nextHeadIndex);
      }

      let f =
          this.setActiveFetch(this.fetcher.get(op, this.nextHeadIndex, opts));
      let logs = await f.p;
      if (logs && logs.length) {
        this.nextHeadIndex = (logs[logs.length - 1].streamIndex + 1);
        this.updateIndexes();
      }
      return logs;
    }

    private async getTail(op: luci.Operation) {
      // If we haven't performed a Tail before, start with one.
      if (this.firstTailIndex < 0) {
        let tidx = this.fetcher.terminalIndex;
        if (tidx < 0) {
          let f = this.setActiveFetch(this.fetcher.getLatest(op));

          let logs = await f.p;

          // Mark our initial "tail" position.
          if (logs && logs.length) {
            this.firstTailIndex = logs[0].streamIndex;
            this.updateIndexes();
          }
          return logs;
        }

        this.firstTailIndex = (tidx + 1);
        this.updateIndexes();
      }

      // We're doing incremental reverse fetches. If we're finished tailing,
      // return no logs.
      if (!this.isSplit()) {
        return [];
      }

      // Determine our walkback region.
      let startIndex = this.firstTailIndex - LogStream.TAIL_WALKBACK;
      if (this.nextHeadIndex >= 0) {
        if (startIndex < this.nextHeadIndex) {
          startIndex = this.nextHeadIndex;
        }
      } else if (startIndex < 0) {
        startIndex = 0;
      }
      let count = (this.firstTailIndex - startIndex);

      // Fetch the full walkback region.
      let f = this.setActiveFetch(this.fetcher.getAll(op, startIndex, count));
      let logs = await f.p;

      this.firstTailIndex = startIndex;
      this.updateIndexes();
      return logs;
    }

    private async getBottom(op: luci.Operation) {
      this.updateIndexes();

      // If there are no more logs in the stream, return no logs.
      if (this.fetchedEndOfStream()) {
        return [];
      }

      // If our bottom index isn't initialized, initialize it via tail.
      if (this.nextBottomIndex < 0) {
        return this.getTail(op);
      }

      let opts = this.nextFetcherOptions();
      let f =
          this.setActiveFetch(this.fetcher.get(op, this.nextBottomIndex, opts));
      let logs = await f.p;
      if (logs && logs.length) {
        this.nextBottomIndex = (logs[logs.length - 1].streamIndex + 1);
      }
      return logs;
    }
  }

  /**
   * LogSorter is an interface that used by AggregateLogStream to extract sorted
   * logs from a set of BufferedLogs.
   *
   * It is used to compare two log entries to determine their relative order.
   */
  type LogSorter = {
    /** Returns true if "a" comes before "b".  */
    before: (a: LogDog.LogEntry, b: LogDog.LogEntry) => number;

    /**
     * If implemented, returns an implicit next log in the buffer set.
     *
     * This is useful if the next log can be determined from the current
     * buffered data, even if it is partial or incomplete.
     */
    implicitNext?: (prev: LogDog.LogEntry, buffers: BufferedLogs[]) =>
                    LogDog.LogEntry | null;
  };

  const prefixIndexLogSorter: LogSorter = {
    before: (a: LogDog.LogEntry, b: LogDog.LogEntry) => {
      return (a.prefixIndex - b.prefixIndex);
    },

    implicitNext:
        (prev: LogDog.LogEntry, buffers: BufferedLogs[]) => {
          let nextPrefixIndex = (prev.prefixIndex + 1);
          for (let buf of buffers) {
            let le = buf.peek();
            if (le && le.prefixIndex === nextPrefixIndex) {
              return buf.next();
            }
          }
          return null;
        },
  };

  const timestampLogSorter: LogSorter = {
    before: (a: LogDog.LogEntry, b: LogDog.LogEntry) => {
      if (a.timestamp) {
        if (b.timestamp) {
          return a.timestamp.getTime() - b.timestamp.getTime();
        }
        return 1;
      }
      if (b.timestamp) {
        return -1;
      }
      return 0;
    },

    // No implicit "next" with timestamp-based logs, since the next log in
    // an empty buffer may actually be the next contiguous log.
    implicitNext: undefined,
  };

  /**
   * An aggregate log stream. It presents a single-stream view, but is really
   * composed of several log streams interleaved based on their prefix indices
   * (if they share a prefix) or timestamps (if they don't).
   *
   * At least one log entry from each stream must be buffered before any log
   * entries can be yielded, since we don't know what ordering to apply
   * otherwise.  To make this fast, we will make the first request for each
   * stream small so it finishes quickly and we can start rendering. Subsequent
   * entries will be larger for efficiency.
   *
   * @param {LogStream} streams the composite streams.
   */
  class AggregateLogStream implements LogProvider {
    private streams: AggregateLogStream.Entry[];
    private active: AggregateLogStream.Entry[];
    private currentNextPromise: Promise<BufferedLogs[]>|null;
    private readonly logSorter: LogSorter;

    private streamStatusCallback: StreamStatusCallback;

    constructor(streams: LogStream[]) {
      // Input streams, ordered by input order.
      this.streams = streams.map<AggregateLogStream.Entry>((ls, i) => {
        ls.setStreamStatusCallback((st: LogStreamStatus[]) => {
          if (st) {
            this.streams[i].status = st[0];
            this.statusChanged();
          }
        });

        return new AggregateLogStream.Entry(ls);
      });

      // Subset of input streams that are still active (not finished).
      this.active = this.streams;

      // The currently-active "next" promise.
      this.currentNextPromise = null;

      // Determine our log comparison function. If all of our logs share a
      // prefix, we will use the prefix index. Otherwise, we will use the
      // timestamp.
      let template: LogDog.StreamPath;
      let sharedPrefix = this.streams.every((entry) => {
        if (!template) {
          template = entry.ls.stream;
          return true;
        }
        return template.samePrefixAs(entry.ls.stream);
      });

      if (sharedPrefix) {
        this.logSorter = prefixIndexLogSorter;
      } else {
        this.logSorter = timestampLogSorter;
      }
    }

    split(): SplitLogProvider|null {
      return null;
    }
    fetchedEndOfStream(): boolean {
      return (!this.active.length);
    }

    setStreamStatusCallback(cb: StreamStatusCallback) {
      this.streamStatusCallback = cb;
    }

    private statusChanged() {
      if (this.streamStatusCallback) {
        // Iterate through our composite stream statuses and pick the one that
        // we want to report.
        this.streamStatusCallback(this.streams.map((entry): LogStreamStatus => {
          return entry.status;
        }));
      }
    }

    getLogStreamUrl(): string|undefined {
      // Return the first log stream viewer URL. IF we have a shared prefix,
      // this will always work. Otherwise, returning something is better than
      // nothing, so if any of the base streams have a URL, we will return it.
      for (let s of this.streams) {
        let url = s.ls.getLogStreamUrl();
        if (url) {
          return url;
        }
      }
      return undefined;
    }

    /**
     * Implements LogProvider.next
     */
    async fetch(op: luci.Operation, _: Location) {
      // If we're already are fetching the next buffer, this is an error.
      if (this.currentNextPromise) {
        throw new Error('In-progress next(), cannot start another.');
      }

      // Filter out any finished streams from our active list. A stream is
      // finished if it is finished streaming and we don't have a retained
      // buffer from it.
      //
      // This updates our "finished" property, since it's derived from the
      // length of our active array.
      this.active = this.active.filter((entry) => {
        return (
            (!entry.buffer) || entry.buffer.peek() ||
            (!entry.ls.fetchedEndOfStream()));
      });

      if (!this.active.length) {
        // No active streams, so we're finished. Permanently set our promise to
        // the finished state.
        return new BufferedLogs(null);
      }

      let buffers: BufferedLogs[];
      this.currentNextPromise = this.ensureActiveBuffers(op);
      try {
        buffers = await this.currentNextPromise;
      } finally {
        this.currentNextPromise = null;
      }
      return this._aggregateBuffers(buffers);
    }

    private async ensureActiveBuffers(op: luci.Operation) {
      // Fill all buffers for all active streams. This may result in an RPC to
      // load new buffer content for streams whose buffers are empty.
      await Promise.all(this.active.map((entry) => entry.ensure(op)));

      // Examine the error status of each stream.
      //
      // The error is interesting, since we must present a common error view to
      // our caller. If all returned errors are "NOT_AUTHENTICATED", we will
      // return a NOT_AUTHENTICATED. Otherwise, we will return a generic
      // "streams failed" error.
      //
      // The outer Promise will pull logs for any streams that don't have any.
      // On success, the "buffer" for the entry will be populated. On failure,
      // an error will be returned. Because Promise.all fails fast, we will
      // catch inner errors and return them as values (null if no error).
      let buffers = new Array<BufferedLogs>(this.active.length);
      let errors = new Array<Error>();
      this.active.forEach((entry, idx) => {
        buffers[idx] = entry.buffer;
        if (entry.lastError) {
          errors.push(entry.lastError);
        }
      });

      // We are done, and will return a value.
      this.currentNextPromise = null;
      if (errors.length) {
        throw this._aggregateErrors(errors);
      }
      return buffers;
    }

    private _aggregateErrors(errors: Error[]): Error {
      let isNotAuthenticated = false;
      errors.every((err) => {
        if (!err) {
          return true;
        }
        if (err === NOT_AUTHENTICATED) {
          isNotAuthenticated = true;
          return true;
        }
        isNotAuthenticated = false;
        return false;
      });
      return (
          (isNotAuthenticated) ? (NOT_AUTHENTICATED) :
                                 new Error('Stream Error'));
    }

    private _aggregateBuffers(buffers: BufferedLogs[]): BufferedLogs {
      switch (buffers.length) {
        case 0:
          // No buffers, so no logs.
          return new BufferedLogs(null);
        case 1:
          // As a special case, if we only have one buffer, and we assume that
          // its entries are sorted, then that buffer is a return value.
          return new BufferedLogs(buffers[0].getAll());
        default:
          break;
      }

      // Preload our peek array.
      let peek = new Array<LogDog.LogEntry>(buffers.length);
      peek.length = 0;
      for (let buf of buffers) {
        let le = buf.peek();
        if (!le) {
          // One of our input buffers had no log entries.
          return new BufferedLogs(null);
        }
        peek.push(le);
      }

      // Assemble our aggregate buffer array.
      //
      // As we add log entries, latestAdded will be updated to point to the most
      // recently added LogEntry.
      let entries: LogDog.LogEntry[] = [];
      let latestAdded: LogDog.LogEntry|null = null;
      while (true) {
        // Choose the next stream.
        let earliest = 0;
        for (let i = 1; i < buffers.length; i++) {
          if (this.logSorter.before(peek[i], peek[earliest])) {
            earliest = i;
          }
        }

        // Get the next log from the earliest stream.
        let next = buffers[earliest].next();
        if (next) {
          latestAdded = next;
          entries.push(latestAdded);
        }

        // Repopulate that buffer's "peek" value. If the buffer has no more
        // entries, then we're done this round.
        next = buffers[earliest].peek();
        if (!next) {
          break;
        }
        peek[earliest] = next;
      }

      // One or more of our buffers is exhausted. If we have the ability to load
      // implicit next logs, try and extract more using that.
      if (latestAdded && this.logSorter.implicitNext) {
        while (true) {
          latestAdded = this.logSorter.implicitNext(latestAdded, buffers);
          if (!latestAdded) {
            break;
          }
          entries.push(latestAdded);
        }
      }
      return new BufferedLogs(entries);
    }
  }

  /** Internal namespace for AggregateLogStream types. */
  namespace AggregateLogStream {
    /** Entry is an entry for a single log stream and its buffered logs. */
    export class Entry {
      buffer = new BufferedLogs(null);
      status: LogStreamStatus;
      lastError: Error|null;

      constructor(readonly ls: LogStream) {
        this.status = ls.getStreamStatus();
      }

      get active() {
        return (
            (!this.buffer) || this.buffer.peek() ||
            this.ls.fetchedEndOfStream());
      }

      async ensure(op: luci.Operation) {
        this.lastError = null;
        if (this.buffer && this.buffer.peek()) {
          return;
        }

        try {
          this.buffer = await this.ls.fetch(op, Location.HEAD);
        } catch (e) {
          // Log stream source of error. Raise a generic "failed to
          // buffer" error. This will become a permanent failure.
          console.error(
              'Error loading buffer for', this.ls.stream.fullName(), '(',
              this.ls, '): ', e);
          this.lastError = e;
        }
      }
    };
  }

  /**
   * A buffer of ordered log entries.
   *
   * Assumes total ownership of the input log buffer, which can be null to
   * indicate no logs.
   */
  class BufferedLogs {
    private index = 0;

    constructor(private logs: LogDog.LogEntry[]|null) {}

    /**
     * Peek returns the next log in the buffer without modifying the buffer. If
     * there are no logs in the buffer, peek will return null.
     */
    peek(): LogDog.LogEntry|null {
      return (this.logs) ? (this.logs[this.index]) : (null);
    }

    /**
     * Returns a copy of the remaining logs in the buffer.
     * If there are no logs, an empty array will be returned.
     */
    peekAll(): LogDog.LogEntry[] {
      return (this.logs || []).slice(0);
    }

    /**
     * GetAll returns all logs in the buffer. Afterwards, the buffer will be
     * empty.
     */
    getAll(): LogDog.LogEntry[] {
      // Pop all logs.
      let logs = this.logs;
      this.logs = null;
      return (logs || []);
    }

    /**
     * Next fetches the next log in the buffer, removing it from the buffer. If
     * no more logs are available, it will return null.
     */
    next(): LogDog.LogEntry|null {
      if (!(this.logs && this.logs.length)) {
        return null;
      }

      // Get the next log and increment our index.
      let log = this.logs[this.index++];
      if (this.index >= this.logs.length) {
        this.logs = null;
      }
      return log;
    }
  }
}
