/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

import {Fetcher, FetcherOptions, FetcherStatus}
    from "logdog-stream-view/fetcher";
import {LogDogQuery, QueryParams, StreamType} from "logdog-stream-view/query";
import {LogDog} from "logdog-stream/logdog";
import * as luci_sleep_promise from "luci-sleep-promise/promise";
import {luci_rpc} from "rpc/client";

/** Sentinel error: not authenticated. */
let NotAuthenticatedError = new Error("Not Authenticated");

function resolveErr(err: Error) {
  let grpc = luci_rpc.GrpcError.convert(err);
  if ( grpc && grpc.code == luci_rpc.Code.UNAUTHENTICATED ) {
    return NotAuthenticatedError;
  }
  return err;
}

/** Stream status entry, as rendered by the view. */
type StreamStatusEntry = {
  name: string;
  desc: string;
};

/** An individual log stream's status. */
type LogStreamStatus = {
  stream: LogDog.Stream;
  state: string;
  fetchStatus: FetcherStatus;
  finished: boolean;
  needsAuth: boolean;
}

type StreamStatusCallback = (v: LogStreamStatus[]) => void;

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

export enum LoadingState {
  NONE,
  RESOLVING,
  LOADING,
  RENDERING,
  NEEDS_AUTH,
  ERROR,
}

/** Represents control visibility in the view. */
type Controls = {
  /** Are we completely finished loading stream data? */
  canSplit: boolean;
  /** Are we currently split? */
  split: boolean;
  /** Show the bottom bar? */
  bottom: boolean;
  /** Is the content fully loaded? */
  fullyLoaded: boolean;

  /** Text in the status bar. */
  loadingState: LoadingState;
  /** Stream status entries, or null for no status window. */
  streamStatus: StreamStatusEntry[];
}

/** Registered callbacks from the LogDog stream view. */
type ViewBinding = {
  client: luci_rpc.Client;
  mobile: boolean;

  pushLogEntries: (entries: LogDog.LogEntry[], l: Location) => void;
  clearLogEntries: () => void;

  updateControls: (c: Controls) => void;
  locationIsVisible: (l: Location) => boolean;
};

/** Interface of the specific Model functions used by the view. */
interface ModelInterface {
  fetch(cancel: boolean): Promise<void>;
  split(): Promise<void>;

  reset(): void;
  setAutomatic(v: boolean): void;
  setTailing(v: boolean): void;
  notifyAuthenticationChanged(): void;
}

export class Model implements ModelInterface {
  /** If performing a small initial fetch, this is the size of the fetch. */
  private static SMALL_INITIAL_FETCH_SIZE = (1024 * 4);
  /** If performing a large initial fetch, this is the size of the fetch. */
  private static LARGE_INITIAL_FETCH_SIZE = (1024 * 24);
  /** If fetching on a mobile device, fetch in this chunk size. */
  private static MOBILE_FETCH_SIZE = (1024 * 256);
  /** For standard fetching, fetch with this size. */
  private static STANDARD_FETCH_SIZE = (4 * 1024 * 1024);

  /**
   * If >0, the maximum number of log lines to push at a time. We will sleep
   * in between these entries to allow the rest of the app to be responsive
   * during log dumping.
   */
  private static logAppendInterval = 4000;
  /** Amount of time to sleep in between log append chunks. */
  private static logAppendDelay = 0;

  /** Our log provider. */
  private provider: LogProvider = this.nullProvider();

  /**
   * Promise that is resolved when authentication state changes. When this
   * happens, a new Promise is installed, and future authentication changes
   * will resolve the new Promise.
   */
  private authChangedPromise: Promise<void> = null;
  /** 
   * Retained callback (Promise resolve) to invoke when authentication state
   * changes.
   */
  private authChangedCallback: (() => void) = null;

  /** The current fetch Promise. */
  private currentFetch: Promise<void>;
  /** The current fetch token. */
  private currentFetchToken: FetchToken;

  /** Are we in automatic mode? */
  private automatic = false;
  /** Are we tailing? */
  private tailing = false;
  /** Are we in the middle of rendering logs? */
  private rendering = true;

  private _loadingState: LoadingState = LoadingState.NONE;
  private _streamStatus: StreamStatusEntry[];

  /**
   * When rendering a Promise that will resolve when the render completes. We
   * use this to pipeline parallel data fetching and rendering.
   */
  private renderPromise: Promise<void>;

  constructor(readonly view: ViewBinding) {
    this.resetAuthChanged();
  }

  resolve(paths: string[]): Promise<void> {
    this.reset();

    // For any path that is a query, execute that query.
    this.loadingState = LoadingState.RESOLVING;
    return Promise.all( paths.map( (path): Promise<LogDog.Stream[]> => {
      let stream = LogDog.Stream.splitProject(path);
      if ( ! LogDogQuery.isQuery(stream.path) ) {
        return Promise.resolve([stream]);
      }

      // This "path" is really a query. Construct and execute.
      let query = new LogDogQuery(this.view.client);
      let doQuery = (): Promise<LogDog.Stream[]> => {
        return query.getAll({
          project: stream.project,
          path: stream.path,
          streamType: StreamType.TEXT,
        }, 100).then( (result): LogDog.Stream[] => {
          return result.map( (qr): LogDog.Stream => {
            return qr.stream;
          } );
        }).catch( (err: Error) => {
          err = resolveErr(err);
          if ( err == NotAuthenticatedError ) {
            return this.authChangedPromise.then( () => {
              return doQuery();
            } );
          }

          throw err;
        });
      }
      return doQuery();
    } ) ).then( (streamBlocks) => {
      let streams = new Array<LogDog.Stream>();
      (streamBlocks || []).forEach( (streamBlock) => {
        streams.push.apply(streams, streamBlock);
      } );


      let initialFetchSize = ( (streams.length === 1) ?
          Model.LARGE_INITIAL_FETCH_SIZE : Model.SMALL_INITIAL_FETCH_SIZE );

      // Determine our fetch size.
      let maxFetchSize = ( (this.view.mobile) ?
          Model.MOBILE_FETCH_SIZE : Model.STANDARD_FETCH_SIZE );

      // Generate a LogStream client entry for each composite stream.
      let logStreams = streams.map( (stream) => {
        console.log("Resolved log stream:", stream);
        return new LogStream(
          this.view.client, stream, initialFetchSize, maxFetchSize);
      });

      // Reset any existing state.
      this.reset();

      // If we have exactly one stream, then use it directly. This allows it to
      // split.
      let provider: LogProvider;
      switch( logStreams.length ) {
      case 0:
        provider = this.nullProvider();
        break;
      case 1:
        provider = logStreams[0];
        break;
      default:
        provider = new AggregateLogStream(logStreams);
        break
      }
      provider.setStreamStatusCallback((st: LogStreamStatus[]) => {
        if ( this.provider === provider ) {
          this.streamStatus = this.buildStreamStatus(st);
        }
      });
      this.provider = provider;
      this.loadingState = LoadingState.NONE;
    } ).catch( (err: Error) => {
      this.loadingState = LoadingState.ERROR;
      console.error("Failed to resolve log streams:", err);
    });
  }

  reset() {
    this.view.clearLogEntries();
    this.clearCurrentFetch();
    this.provider = this.nullProvider();

    this.updateControls();
  }

  private nullProvider(): LogProvider { return new AggregateLogStream([]); }

  private mintFetchToken(): FetchToken {
    return (this.currentFetchToken = new FetchToken( (tok: FetchToken) => {
      return (tok === this.currentFetchToken);
    } ));
  }

  private clearCurrentFetch() {
    this.currentFetch = this.currentFetchToken = null;
    this.rendering = false;
  }

  private get loadingState(): LoadingState { return this._loadingState; }
  private set loadingState(v: LoadingState) {
    if( v != this._loadingState ) {
      this._loadingState = v;
      this.updateControls();
    }
  }

  private get streamStatus(): StreamStatusEntry[] { return this._streamStatus; }
  private set streamStatus(st: StreamStatusEntry[]) {
    this._streamStatus = st;
    this.updateControls();
  }

  private updateControls() {
    this.view.updateControls({
      canSplit: this.providerCanSplit,
      split: this.isSplit,
      bottom: !this.fetchedEndOfStream,
      fullyLoaded: (this.fetchedFullStream && (! this.rendering)),
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
    if ( this.authChangedCallback ) {
      this.authChangedCallback();
    }

    // Create a new Promise and install it.
    this.authChangedPromise = new Promise<void>((resolve, reject) => {
      this.authChangedCallback = resolve;
    });
  }

  split(): Promise<void> {
    // If we haven't already split, and our provider lets us split, then go
    // ahead and do so.
    if ( this.providerCanSplit ) {
      return this.fetchLocation(Location.TAIL, true);
    }
    return this.fetch(false);
  }

  fetch(cancel: boolean): Promise<void> {
    if ( this.isSplit ) {
      if ( this.tailing ) {
        // Next fetch grabs logs from the bottom (continue tailing).
        if ( ! this.fetchedEndOfStream ) {
          return this.fetchLocation(Location.BOTTOM, false);
        } else {
          return this.fetchLocation(Location.TAIL, false);
        }
      }

      // We're split, but not tailing, so fetch logs from HEAD.
      return this.fetchLocation(Location.HEAD, false);
    }

    // We're not split. If we haven't reached end of stream, fetch logs from
    // HEAD.
    return this.fetchLocation(Location.HEAD, false);
  }

  /** Fetch logs from an explicit location. */
  fetchLocation(l: Location, cancel: boolean) {
    if ( this.currentFetch && (!cancel) ) {
      return this.currentFetch;
    }

    // If our provider is finished, then do nothing.
    if ( this.fetchedFullStream ) {
      // There are no more logs.
      return Promise.resolve(null);
    }

    // If we're asked to fetch BOTTOM, but we're not split, fetch HEAD instead.
    if ( l === Location.BOTTOM && ! this.isSplit ) {
      l = Location.HEAD;
    }

    // If we're not split, always fetch from BOTTOM.
    this.loadingState = LoadingState.LOADING;

    // Rotate our fetch ID. This will effectively cancel any pending fetches.
    let token = this.mintFetchToken();
    return (this.currentFetch = this.provider.fetch(l, token).then( (buf) => {
      // Clear our fetching status.
      this.rendering = true;
      this.loadingState = LoadingState.RENDERING;
      let pushLogsPromise: Promise<void>;
      let hasLogs = (buf && buf.peek());

      // Resolve any previous rendering Promise that we have. This makes sure
      // our rendering and fetching don't get more than one round out of sync.
      return (this.renderPromise || Promise.resolve(null)).then( () => {
        // Post-fetch cleanup.
        this.clearCurrentFetch();

        // Clear our loading state (updates controls automatically).
        this.loadingState = LoadingState.RENDERING;

        // Initiate the next render. This will happen in the background while
        // we enqueue our next fetch.
        this.renderPromise = this.renderLogs(buf, l).then( () => {
          this.renderPromise = null;
          if ( this.loadingState === LoadingState.RENDERING ) {
            this.loadingState = LoadingState.NONE;
          }
        });

        if ( this.fetchedFullStream ) {
          // If we're finished now, perform our finished cleanup.
          return;
        }

        // The fetch is finished. If we're automatic, and we got logs, start the
        // next.
        if ( this.automatic && hasLogs ) {
          console.log("Automatic: starting next fetch.")
          return this.fetch(false);
        }
      });
    }).catch( (err: Error) => {
      // If we've been canceled, discard this result.
      if ( ! token.valid ) {
        return
      }

      this.clearCurrentFetch();
      if ( err === NotAuthenticatedError ) {
        this.loadingState = LoadingState.NEEDS_AUTH;

        // We failed because we were not authenticated. Mark this so we can
        // retry if that state changes.
        return this.authChangedPromise.then( () => {
          // Our authentication state changed during the fetch! Retry
          // automatically.
          this.fetchLocation(l, false);
        });
      }

      console.error("Failed to load log streams:", err);
    }));
  }

  private renderLogs(buf: BufferedLogs, l: Location): Promise<void> {
    if ( ! (buf && buf.peek()) ) {
      return Promise.resolve(null);
    }

    let lines = 0;
    let logBlock = new Array<LogDog.LogEntry>();
    let appendBlock = () => {
      if ( logBlock.length ) {
        console.log("Rendering", logBlock.length, "logs...");
        this.view.pushLogEntries(logBlock, l);
        logBlock.length = 0;
        lines = 0;

        // Update our status and controls.
        this.updateControls();
      }
    };

    // Create a promise loop to push logs at intervals.
    let pushLogs = (): Promise<void> => {
      return Promise.resolve().then( () => {
        // Add logs until we reach our interval lines.
        for ( let nextLog = buf.next(); (nextLog); nextLog = buf.next() ) {
          // If we've exceeded our burst, then interleave a sleep (yield).
          if (Model.logAppendInterval > 0 &&
              lines >= Model.logAppendInterval ) {
            appendBlock();

            return luci_sleep_promise.sleep(Model.logAppendDelay).then(
              () => {
                // Enqueue the next push round.
                return pushLogs();
              } );
          }

          // Add the next log to the append block.
          logBlock.push(nextLog);
          if ( nextLog.text && nextLog.text.lines ) {
            lines += nextLog.text.lines.length;
          }
        }

        // If there are any buffered logs, append that block.
        appendBlock();
      });
    }
    return pushLogs();
  }

  setTailing(v: boolean) {
    this.tailing = v;
  }

  setAutomatic(v: boolean) {
    this.automatic = v;
    if ( v ) {
      // Passively kick off a new fetch.
      this.fetch(false);
    }
  }

  private buildStreamStatus(v: LogStreamStatus[]): StreamStatusEntry[] {
    let maxStatus = FetcherStatus.IDLE;
    let maxStatusCount = 0;
    let needsAuth = false;

    // Prune any finished entries and accumulate them for status bar change.
    v = (v || []).filter( (st) => {
      needsAuth = (needsAuth || st.needsAuth);

      if ( st.fetchStatus > maxStatus ) {
        maxStatus = st.fetchStatus;
        maxStatusCount = 1;
      } else if ( st.fetchStatus === maxStatus ) {
        maxStatusCount++;
      }

      return (! st.finished);
    });

    return v.map( (st): StreamStatusEntry => {
      return {
        name: ".../+/" + st.stream.name,
        desc: st.state,
      };
    } );
  }

  private get providerCanSplit(): boolean {
    let split = this.provider.split();
    return (!! (split && split.canSplit()));
  }

  private get isSplit(): boolean {
    let split = this.provider.split();
    return ( !! (split && split.isSplit()) );
  }

  private get fetchedEndOfStream(): boolean {
    return (this.provider.fetchedEndOfStream());
  }

  private get fetchedFullStream(): boolean {
    return (this.fetchedEndOfStream && (! this.isSplit));
  }
}

/**
 * A token used to repesent an individual fetch. A token can assert whether its
 * fetch has been invalidated.
 */
class FetchToken {
  private validate: (tok: FetchToken) => boolean;

  constructor(validate: (tok: FetchToken) => boolean) {
    this.validate = validate;
  }

  get valid(): boolean {
    return this.validate(this);
  }

  do<T>(p: Promise<T>): Promise<T> {
    return p.then( (v): T => {
      if ( ! this.valid ) {
        throw new Error("Token has been invalidated, discarding fetch.");
      }
      return v
    } );
  }
}

interface LogProvider {
  setStreamStatusCallback(cb: StreamStatusCallback): void;
  fetch(l: Location, token: FetchToken): Promise<BufferedLogs>;

  /** Will return null if this LogProvider doesn't support splitting. */
  split(): SplitLogProvider;
  fetchedEndOfStream(): boolean;
}

/** Additional methods for log stream splitting, if supported. */
interface SplitLogProvider {
  canSplit(): boolean
  isSplit(): boolean;
}

/** A LogStream is a LogProvider manages a single log stream. */
class LogStream implements LogProvider {
  /**
   * Always begin with a small fetch. We'll disable this afterward the first
   * finishes.
   */
  private initialFetch = true;

  private fetcher: Fetcher;

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

  constructor(client: luci_rpc.Client, readonly stream: LogDog.Stream,
              readonly initialFetchSize: number,
              readonly maxFetchSize: number ) {
    this.fetcher = new Fetcher(client, stream);
    this.fetcher.setStatusChangedCallback( () => {
      this.statusChanged();
    });
  }

  get fetcherStatus(): FetcherStatus { return this.fetcher.status; }

  fetch(l: Location, token: FetchToken): Promise<BufferedLogs> {
    // Determine which method to use based on the insertion point and current
    // log stream fetch state.
    let getLogs: Promise<LogDog.LogEntry[]>;
    switch( l ) {
    case Location.HEAD:
      getLogs = this.getHead(token);
      break;

    case Location.TAIL:
      getLogs = this.getTail(token);
      break;

    case Location.BOTTOM:
      getLogs = this.getBottom(token);
      break;
    }

    return getLogs.then( (logs: LogDog.LogEntry[]) => {
      this.initialFetch = false;
      this.statusChanged();
      return new BufferedLogs(logs);
    }).catch( (err: Error) => {
      err = resolveErr(err);
      throw err;
    });
  }

  setStreamStatusCallback(cb: StreamStatusCallback) {
    this.streamStatusCallback = cb;
  }

  private statusChanged() {
    if ( this.streamStatusCallback ) {
      this.streamStatusCallback([this.getStreamStatus()]);
    }
  }

  getStreamStatus(): LogStreamStatus {
    let pieces = new Array<string>();
    let tidx = this.fetcher.terminalIndex;
    if ( this.nextHeadIndex > 0 ) {
      pieces.push("1.." + this.nextHeadIndex);
    } else {
      pieces.push("0");
    }
    if ( this.isSplit() ) {
      if ( tidx >= 0 ) {
        pieces.push("| " + this.firstTailIndex + " / " + tidx);
        tidx = -1;
      } else {
        pieces.push("| " + this.firstTailIndex + ".." + this.nextBottomIndex +
                    " ...");
      }
    } else if (tidx >= 0) {
      pieces.push("/ " + tidx);
    } else {
      pieces.push("...");
    }

    let needsAuth = false;
    let finished = this.finished;
    if ( finished ) {
      pieces.push("(Finished)");
    } else {
      switch ( this.fetcher.status ) {
      case FetcherStatus.IDLE:
      case FetcherStatus.LOADING:
        pieces.push("(Loading)");
        break;

      case FetcherStatus.STREAMING:
        pieces.push("(Streaming)");
        break;

      case FetcherStatus.MISSING:
        pieces.push("(Missing)");
        break;

      case FetcherStatus.ERROR:
        let err = resolveErr(this.fetcher.lastError);
        if (err === NotAuthenticatedError ) {
          pieces.push("(Auth Error)");
          needsAuth = true;
        } else {
          pieces.push("(Error)");
        }
        break;
      }
    }

    return {
      stream: this.stream,
      state: pieces.join(" "),
      finished: finished,
      fetchStatus: this.fetcher.status,
      needsAuth: needsAuth,
    };
  }

  split(): SplitLogProvider {
    return this;
  }

  isSplit(): boolean {
    // We're split if we have a bottom and we're not finished tailing.
    return ( this.firstTailIndex >= 0 &&
        (this.nextHeadIndex < this.firstTailIndex) );
  }

  canSplit(): boolean {
    return ( ! (this.isSplit() || this.caughtUp) );
  }

  private get caughtUp(): boolean {
    // We're caught up if we have both a head and bottom index, and the head
    // is at or past the bottom.
    return ( this.nextHeadIndex >= 0 && this.nextBottomIndex >= 0 &&
            this.nextHeadIndex >= this.nextBottomIndex );
  }

  fetchedEndOfStream(): boolean {
    let tidx = this.fetcher.terminalIndex;
    return ( tidx >= 0 && (
      (this.nextHeadIndex > tidx) || (this.nextBottomIndex > tidx) ) );
  }

  private get finished(): boolean {
    return ( (! this.isSplit()) && this.fetchedEndOfStream() );
  }

  private updateIndexes() {
    if ( this.firstTailIndex >= 0 ) {
      if ( this.nextBottomIndex < this.firstTailIndex ) {
        this.nextBottomIndex = this.firstTailIndex + 1;
      }

      if ( this.nextHeadIndex >= this.firstTailIndex &&
          this.nextBottomIndex >= 0) {
        // Synchronize our head and bottom pointers.
        this.nextHeadIndex = this.nextBottomIndex =
            Math.max(this.nextHeadIndex, this.nextBottomIndex);
      }
    }
  }

  private nextFetcherOptions(): FetcherOptions {
    let opts: FetcherOptions = {};
    if ( this.initialFetch ) {
      opts.byteCount = this.initialFetchSize;
    } else if ( this.maxFetchSize > 0 ) {
      opts.byteCount = this.maxFetchSize;
    }
    return opts;
  }

  private getHead(token: FetchToken): Promise<LogDog.LogEntry[]> {
    this.updateIndexes();

    if ( this.finished ) {
      // Our HEAD region has met/surpassed our TAIL region, so there are no
      // HEAD logs to return. Only bottom.
      return Promise.resolve();
    }

    // If we have a tail pointer, only fetch HEAD up to that point.
    let opts = this.nextFetcherOptions();
    if ( this.firstTailIndex >= 0 ) {
      opts.logCount = (this.firstTailIndex - this.nextHeadIndex);
    }

    return token.do( this.fetcher.get(this.nextHeadIndex, opts) ).then(
      (logs) => {
        if ( logs && logs.length ) {
          this.nextHeadIndex = (logs[logs.length - 1].streamIndex + 1);
          this.updateIndexes();
        }
        return logs;
      } );
  }

  private getTail(token: FetchToken): Promise<LogDog.LogEntry[]> {
    // If we haven't performed a Tail before, start with one.
    if ( this.firstTailIndex < 0 ) {
      let tidx = this.fetcher.terminalIndex;
      if ( tidx < 0 ) {
        return token.do( this.fetcher.tail() ).then( (logs) => {
          // Mark our initial "tail" position.
          if ( logs && logs.length ) {
            this.firstTailIndex = logs[0].streamIndex;
            this.updateIndexes();
          }
          return logs;
        } );
      }

      this.firstTailIndex = (tidx+1);
      this.updateIndexes();
    }

    // We're doing incremental reverse fetches. If we're finished tailing,
    // return no logs.
    if ( ! this.isSplit() ) {
      return Promise.resolve(null);
    }

    // Determine our walkback region.
    let startIndex = this.firstTailIndex - LogStream.TAIL_WALKBACK;
    if ( this.nextHeadIndex >= 0 ) {
      if ( startIndex < this.nextHeadIndex ) {
        startIndex = this.nextHeadIndex;
      }
    } else if ( startIndex < 0 ) {
      startIndex = 0;
    }
    let count = (this.firstTailIndex - startIndex);

    // Fetch the full walkback region.
    return token.do( this.fetcher.getAll(startIndex, count) ).then( (logs) => {
      this.firstTailIndex = startIndex;
      this.updateIndexes();
      return logs;
    });
  }

  private getBottom(token: FetchToken): Promise<LogDog.LogEntry[]> {
    this.updateIndexes();

    // If there are no more logs in the stream, return no logs.
    if ( this.fetchedEndOfStream() ) {
      return Promise.resolve(null);
    }

    // If our bottom index isn't initialized, initialize it via tail.
    if ( this.nextBottomIndex < 0 ) {
      return this.getTail(token);
    }

    let opts = this.nextFetcherOptions();
    return token.do( this.fetcher.get(this.nextBottomIndex, opts) ).then(
      (logs) => {
        if ( logs && logs.length ) {
          this.nextBottomIndex = (logs[logs.length - 1].streamIndex + 1);
        }
        return logs;
      } );
  }
}

/**
 * An aggregate log stream. It presents a single-stream view, but is really
 * composed of several log streams interleaved based on their prefix indices
 * (if they share a prefix) or timestamps (if they don't).
 *
 * At least one log entry from each stream must be buffered before any log
 * entries can be yielded, since we don't know what ordering to apply otherwise.
 * To make this fast, we will make the first request for each stream small so
 * it finishes quickly and we can start rendering. Subsequent entries will be
 * larger for efficiency.
 *
 * @param {LogStream} streams the composite streams.
 */
class AggregateLogStream implements LogProvider {

  private streams: AggregateLogStream.Entry[];
  private active: AggregateLogStream.Entry[];
  private currentNextPromise: Promise<BufferedLogs>;
  private compareLogs: (a: LogDog.LogEntry, b: LogDog.LogEntry) => number;

  private streamStatusCallback: StreamStatusCallback;

  constructor(streams: LogStream[]) {
    // Input streams, ordered by input order.
    this.streams = streams.map<AggregateLogStream.Entry>( (ls, i) => {
      ls.setStreamStatusCallback( (st: LogStreamStatus[]) => {
        if ( st ) {
          this.streams[i].status = st[0];
          this.statusChanged();
        }
      });

      return {
        ls: ls,
        buffer: null,
        needsAuth: false,
        status: ls.getStreamStatus(),
      };
    } );

    // Subset of input streams that are still active (not finished).
    this.active = this.streams;

    // The currently-active "next" promise.
    this.currentNextPromise = null;

    // Determine our log comparison function. If all of our logs share a prefix,
    // we will use the prefix index. Otherwise, we will use the timestamp.
    let template: LogDog.Stream = null;
    let sharedPrefix = this.streams.every( (entry) => {
      if ( ! template ) {
        template = entry.ls.stream;
        return true;
      }
      return template.samePrefixAs(entry.ls.stream);
    });

    this.compareLogs = (( sharedPrefix ) ?
        (a, b) => {
          return (a.prefixIndex - b.prefixIndex);
        } :
        (a, b) => {
          return a.timestamp.getTime() - b.timestamp.getTime();
        });
  }

  split(): SplitLogProvider { return null; }
  fetchedEndOfStream(): boolean { return ( ! this.active.length ); }

  setStreamStatusCallback(cb: StreamStatusCallback) {
    this.streamStatusCallback = cb;
  }

  private statusChanged() {
    if ( this.streamStatusCallback ) {
      // Iterate through our composite stream statuses and pick the one that we
      // want to report.
      this.streamStatusCallback( this.streams.map( (entry): LogStreamStatus => {
        return entry.status;
      } ));
    }
  }

  /**
   * Implements LogProvider.next
   */
  fetch(l: Location, token: FetchToken): Promise<BufferedLogs> {
    // If we're already are fetching the next buffer, this is an error.
    if (this.currentNextPromise) {
      throw new Error("In-progress next(), cannot start another.");
    }

    // Filter out any finished streams from our active list. A stream is
    // finished if it is finished streaming and we don't have a retained buffer
    // from it.
    //
    // This updates our "finished" property, since it's derived from the length
    // of our active array.
    this.active = this.active.filter( (entry) => {
      return ( (! entry.buffer) || entry.buffer.peek() ||
              (! entry.ls.fetchedEndOfStream()) );
    });

    if ( ! this.active.length ) {
      // No active streams, so we're finished. Permanently set our promise to
      // the finished state.
      return Promise.resolve();
    }

    // Fill all buffers for all active streams. This may result in an RPC to
    // load new buffer content for streams whose buffers are empty.
    //
    // If any stream doesn't currently have buffered logs, we will call their
    // "next()" methods to pull the next set of logs. This will result in one of
    // three possibilities:
    // - A BufferedLogs will be returned containing the next logs for this stream.
    //   The log stream may also be finished.
    // - null will be returned, and this log stream must now be finished.
    // - An error will be returned.
    //
    // The error is interesting, since we must present a common error view to our
    // caller. If all returned errors are "NotAuthenticatedError", we will return
    // a NotAuthenticatedError. Otherwise, we will return a generic "streams
    // failed" error.
    //
    // The outer Promise will pull logs for any streams that don't have any.
    // On success, the "buffer" for the entry will be populated. On failure, an
    // error will be returned. Because Promise.all fails fast, we will catch inner
    // errors and return them as values (null if no error).
    this.currentNextPromise = Promise.all( this.active.map( (entry) => {
        // If the entry's buffer still has data, use it immediately.
        if (entry.buffer && entry.buffer.peek()) {
          return null;
        }

        // No buffered logs. Call the stream's "next()" method to get some.
        return entry.ls.fetch(Location.HEAD, token).then(
          (buffer): Error => {
            // Retain this buffer.
            entry.buffer = buffer;
            return null;
          }
        ).catch( (error: Error) => {
          // Log stream source of error. Raise a generic "failed to buffer"
          // error. This will become a permanent failure.
          console.error("Error loading buffer for", entry.ls.stream.fullName(),
              "(", entry.ls, "): ", error);
          return error;
        });
      })).then( (results: Error[]): BufferedLogs => {
        // Identify any errors that we hit.
        let buffers = new Array<BufferedLogs>(this.active.length);
        let errors: Error[] = [];
        results.forEach( (err, idx) => {
          buffers[idx] = this.active[idx].buffer;
          if ( err ) { errors[idx] = err; }
        });

        // We are done, and will return a value.
        this.currentNextPromise = null;
        if ( errors.length ) {
          throw this._aggregateErrors(errors);
        }
        return this._aggregateBuffers(buffers);
      });

    return this.currentNextPromise;
  }

  private _aggregateErrors(errors: Error[]): Error {
    let isNotAuthenticated = false;
    errors.every( (err) => {
      if ( ! err ) { return true; }
      if ( err === NotAuthenticatedError ) {
        isNotAuthenticated = true;
        return true;
      }
      isNotAuthenticated = false;
      return false;
    });
    return (( isNotAuthenticated ) ?
        (NotAuthenticatedError) : new Error("Stream Error"));
  }

  private _aggregateBuffers(buffers: BufferedLogs[]): BufferedLogs {
    switch ( buffers.length ) {
    case 0:
      // No buffers, so no logs.
      return new BufferedLogs(null);
    case 1:
      // As a special case, if we only have one buffer, and we assume that its
      // entries are sorted, then that buffer is a return value.
      return new BufferedLogs(buffers[0].getAll());
    }

    // Preload our peek array.
    let incomplete = false;
    let peek = buffers.map(function(buf) {
      var le = buf.peek();
      if (! le) {
        incomplete = true;
      }
      return le;
    });
    if (incomplete) {
      // One of our input buffers had no log entries.
      return new BufferedLogs(null);
    }

    // Assemble our aggregate buffer array.
    let entries: LogDog.LogEntry[] = [];
    while (true) {
      // Choose the next stream.
      var earliest = 0;
      for (var i = 1; i < buffers.length; i++) {
        if (this.compareLogs(peek[i], peek[earliest]) < 0) {
          earliest = i;
        }
      }

      // Get the next log from the earliest stream.
      entries.push(buffers[earliest].next());

      // Repopulate that buffer's "peek" value. If the buffer has no more
      // entries, then we're done.
      var next = buffers[earliest].peek();
      if (!next) {
        return new BufferedLogs(entries);
      }
      peek[earliest] = next;
    }
  }
}

module AggregateLogStream {
  export type Entry = {
    ls: LogStream;
    buffer: BufferedLogs;
    needsAuth: boolean;
    status: LogStreamStatus;
  }
}

/**
 * A buffer of ordered log entries.
 *
 * Assumes total ownership of the input log buffer, which can be null to
 * indicate no logs.
 */
class BufferedLogs {
  private logs: LogDog.LogEntry[] | null;
  private index: number;

  constructor(logs: LogDog.LogEntry[] | null) {
    this.logs = logs;
    this.index = 0;
  }

  peek(): LogDog.LogEntry | null {
    return (this.logs) ? (this.logs[this.index]) : (null);
  }

  getAll(): LogDog.LogEntry[] {
    // Pop all logs.
    var logs = this.logs;
    this.logs = null;
    return logs;
  }

  next() : LogDog.LogEntry | null {
    if (! (this.logs && this.logs.length)) {
      return null;
    }

    // Get the next log and increment our index.
    var log = this.logs[this.index++];
    if (this.index >= this.logs.length) {
      this.logs = null;
    }
    return log;
  }
}
