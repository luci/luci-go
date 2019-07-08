/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

///<reference path="../luci-operation/operation.ts" />
///<reference path="../luci-sleep-promise/promise.ts" />

namespace luci {
  /**
   * Describes the exposed functionality of a single Polymer RPC call, defined
   * in "rpc-call.html".
   */
  interface PolymerClientCall {
    /** Aborts the call, if it is currently ongoing. */
    abort(): void;

    /**
     * Completes performs the configured RPC, returning a Promise that
     * resolves with the call's result on completion and an HTTP/gRPC error on
     * failure.
     */
    completes: Promise<any>;
  }

  /**
   * PolymerClient describes the exposed functionality of the Polymer RPC
   * client,
   * defined in "rpc-client.html".
   *
   * TODO(dnj): Fully port the Polymer RPC client to TypeScript. For now, we'll
   * let it continue to do the request/response logic and piggyback on top of
   * its
   * Promises to provide retries and advanced capabilities.
   */
  export interface PolymerClient {
    /** Name of the RPC service to invoke. */
    service: string;
    /** Name of the RPC method to invoke. */
    method: string;
    /** Request contents. */
    request: any;

    /**
     * Constructs the RPC call, returning its result. Will return an HttpError
     * instance (see "rpc-error.html") on HTTP error, and a GrpcError (see
     * "rpc-call.html") on gRPC error.
     *
     * Call returns an object configured to execute an RPC against the specified
     * service and method.
     */
    call(): PolymerClientCall;
  }

  /**
   * An RPC client implementation.
   *
   * This client implements exponential backoff retries on transient HTTP and
   * gRPC errors.
   */
  export class Client {
    /** Retry instance to use for retries. */
    transientRetry: Retry = {retries: 10, delay: 500, maxDelay: 15000};

    /**
     * Constructs a new Client.
     *
     * @param pc "rpc-client.html" client instance to use for calls.
     */
    constructor(private pc: PolymerClient) {}

    /**
     * Call invokes the specified service's method, returning a Promise.
     *
     * @param service the RPC service name to call.
     * @param method the RPC method name to call.
     * @param request optional request body content.
     *
     * @returns A Promise that resolves to the RPC response, or errors with an
     *     error, including HttpError or GrpcError on HTTP/gRPC failure
     *     respectively.
     */
    async call<T>(service: string, method: string, request?: T) {
      return this.callOp(undefined, service, method, request);
    }

    async callOp<T>(
        op: luci.Operation|undefined, service: string, method: string,
        request?: T) {
      let transientRetry = new RetryIterator(this.transientRetry);
      let currentCall: PolymerClientCall;

      let resp = await transientRetry.do(
          () => {
            if (op && currentCall) {
              // Unregister previous call instances' cancellation callbacks.
              op.removeCancelCallback(currentCall.abort);
            }

            // Configure the client for this request.
            this.pc.service = service;
            this.pc.method = method;
            this.pc.request = request;

            // Execute the configured request.
            //
            // If we are supplied an operation, allow it to cancel the call
            // directly.
            currentCall = this.pc.call();
            if (op) {
              op.addCancelCallback(currentCall.abort);
            }
            return currentCall.completes;
          },
          (err: Error, delay: number) => {
            // Is this a transient error?
            if (!isTransientError(err)) {
              throw err;
            }
            console.warn(
                `Transient error calling ` +
                    `${service}.${method} with params:`,
                request, `:`, err, `; retrying after ${delay}ms.`);
          },
          op);
      return resp.response;
    }
  }

  /**
   * gRPC Codes
   *
   * Copied from "rpc-code.html" and, more directly,
   * https://github.com/grpc/grpc-go/blob/972dbd2/codes/codes.go#L43
   */
  export enum Code {
    OK = 0,
    CANCELED = 1,
    UNKNOWN = 2,
    INVALID_ARGUMENT = 3,
    DEADLINE_EXCEEDED = 4,
    NOT_FOUND = 5,
    ALREADY_EXISTS = 6,
    PERMISSION_DENIED = 7,
    UNAUTHENTICATED = 16,
    RESOURCE_EXHAUSTED = 8,
    FAILED_PRECONDITION = 9,
    ABORTED = 10,
    OUT_OF_RANGE = 11,
    UNIMPLEMENTED = 12,
    INTERNAL = 13,
    UNAVAILABLE = 14,
    DATA_LOSS = 15
  }

  /** A gRPC error, which is an error paired with a gRPC Code. */
  export class GrpcError extends Error {
    constructor(readonly code: Code, readonly description?: string) {
      super('code = ' + code + ', desc = ' + description);
    }

    /**
     * Converts the supplied Error into a GrpcError if its name is
     * "GrpcError". This merges between the non-Typescript RPC code and this
     * error type.
     */
    static convert(err: Error): GrpcError|null {
      if (err.name === 'GrpcError') {
        let aerr = err as any as {code: Code, description: string};
        return new GrpcError(aerr.code, aerr.description);
      }
      return null;
    }

    /** Returns true if the error is considered transient. */
    get transient(): boolean {
      switch (this.code) {
        case Code.INTERNAL:
        case Code.UNAVAILABLE:
        case Code.RESOURCE_EXHAUSTED:
          return true;

        default:
          return false;
      }
    }
  }

  /** An HTTP error, which is an error paired with an HTTP code. */
  export class HttpError extends Error {
    constructor(readonly code: number, readonly description?: string) {
      super('code = ' + code + ', desc = ' + description);
    }

    /**
     * Converts the supplied Error into a HttpError if its name is
     * "HttpError". This merges between the non-Typescript RPC code and this
     * error type.
     */
    static convert(err: Error): HttpError|null {
      if (err.name === 'HttpError') {
        let aerr = err as any as {code: number, description: string};
        return new HttpError(aerr.code, aerr.description);
      }
      return null;
    }

    /** Returns true if the error is considered transient. */
    get transient(): boolean {
      return (this.code >= 500);
    }
  }

  /**
   * Generic error processing function to determine if an error is known to be
   * transient.
   */
  export function isTransientError(err: Error): boolean {
    let grpc = GrpcError.convert(err);
    if (grpc) {
      return grpc.transient;
    }

    // Is this an HTTP Error?
    let http = HttpError.convert(err);
    if (http) {
      return http.transient;
    }

    // Unknown error.
    return false;
  }

  /**
   * RetryIterator configuration class.
   *
   * A user will define the retry parameters using a Retry instance, then create
   * a RetryIterator with them.
   */
  export type Retry = {
    // The number of retries to perform before failing. If undefined, will retry
    // indefinitely.
    retries?: number;

    // The amount of time to delay in between retry attempts, in milliseconds.
    // If undefined or < 0, no delay will be imposed.
    delay: number;
    // The maximum delay to apply, in milliseconds. If > 0 and delay scales past
    // "maxDelay", it will be capped at "maxDelay".
    maxDelay?: number;

    // delayScaling is the multiplier applied to "delay" in between retries. If
    // undefined or <= 1, DEFAULT_DELAY_SCALING will be used.
    delayScaling?: number;
  };

  /** RetryCallback is an optional callback type used in "do". */
  export type RetryCallback = (err: Error, delay: number) => void;

  /**
   * Stopped is a sentinel error thrown by RetryIterator when it runs out of
   * retries.
   */
  export const STOPPED = new Error('retry stopped');

  /**
   * Generic exponential backoff retry delay generator.
   *
   * A RetryIterator is a specific configured instance of a Retry. Each call to
   * "next()" returns the next delay in the retry sequence.
   */
  export class RetryIterator {
    // Default scaling if no delay is specified.
    static readonly DEFAULT_DELAY_SCALING = 2;

    private delay: number;
    private retries: number|undefined;
    private maxDelay: number;
    private delayScaling: number;

    constructor(config: Retry) {
      this.retries = config.retries;
      this.maxDelay = (config.maxDelay || 0);
      this.delay = (config.delay || 0);

      this.delayScaling = (config.delayScaling || 0);
      if (this.delayScaling < 1) {
        this.delayScaling = RetryIterator.DEFAULT_DELAY_SCALING;
      }
    }

    /**
     * @returns the next delay, in milliseconds. If there are no more retries,
     *      returns undefined.
     */
    next(): number {
      // Apply retries, if they have been enabled.
      if (this.retries !== undefined) {
        if (this.retries <= 0) {
          // No more retries remaining.
          throw STOPPED;
        }
        this.retries--;
      }

      let delay = this.delay;
      this.delay *= this.delayScaling;
      if (this.maxDelay > 0 && delay > this.maxDelay) {
        this.delay = delay = this.maxDelay;
      }
      return delay;
    }

    /**
     * Executes a Promise, retrying if the Promise raises an error.
     *
     * "do" iteratively tries to execute a Promise, generated by "gen". If that
     * Promise raises an error, "do" will retry until it either runs out of
     * retries, or the Promise does not return an error. Each retry, "do" will
     * invoke "gen" again to generate a new Promise.
     *
     * An optional "onError" callback can be supplied. If it is, it will be
     * invoked in between each retry. The callback may, itself, throw, in which
     * case the retry loop will stop. This can be used for reporting and/or
     * selective retries.
     *
     * @param gen Promise generator function for retries.
     * @param onError optional callback to be invoked in between retries.
     * @param op if supplied, this Operation will be used to cancel retries.
     *
     * @throws any the error generated by "gen"'s Promise, if out of retries,
     *     or the error raised by onError if it chooses to throw.
     */
    async do<T>(
        gen: () => Promise<T>, onError?: RetryCallback, op?: luci.Operation) {
      let base = this.doImpl(gen, onError);
      if (op) {
        base = op.wrap(base);
      }
      return base;
    }

    private async doImpl<T>(
        gen: () => Promise<T>, onError?: RetryCallback, op?: luci.Operation) {
      while (true) {
        let genErr: Error;
        try {
          return await gen();
        } catch (e) {
          genErr = e;
        }

        let delay: number;
        try {
          delay = this.next();
        } catch (e) {
          if (e !== STOPPED) {
            console.warn('Unexpected error generating next delay:', e);
          }

          // If we could not generate another retry delay, raise the initial
          // Promise's error.
          throw genErr;
        }

        if (onError) {
          // Note: this may throw.
          onError(genErr, delay);
        }
        await luci.sleepPromise(delay, op);
      }
    }
  }
}
