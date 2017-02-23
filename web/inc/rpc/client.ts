/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

///<reference path="../luci-sleep-promise/promise.ts" />

namespace luci {
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
  interface PolymerClient {
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
    call(): {
      /**
       * Completes performs the configured RPC, returning a Promise that
       * resolves with the call's result on completion and an HTTP/gRPC error on
       * failure.
       */
      completes: Promise<any>,
    };
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
    call<T, R>(service: string, method: string, request?: T): Promise<R> {
      let transientRetry = new RetryIterator(this.transientRetry);
      let doCall = (): Promise<R> => {
        // Configure the client for this request.
        this.pc.service = service;
        this.pc.method = method;
        this.pc.request = request;

        // Execute the configured request.
        let callPromise: Promise<R> = this.pc.call().completes;
        return callPromise.then((resp: any) => resp.response)
            .catch((err: Error) => {
              // Is this a transient error?
              if (isTransientError(err)) {
                let delay = transientRetry.next();
                if (delay >= 0) {
                  console.warn(
                      `Transient error calling ` +
                          `${service}.${method} with params:`,
                      request, `:`, err, `; retrying after ${delay}ms.`);
                  return luci.sleepPromise(delay).then(doCall);
                }
              }

              // Non-transient, throw the error.
              throw err;
            });
      };
      return doCall();
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
    retries: number | undefined;

    // The amount of time to delay in between retry attempts, in milliseconds.
    // If undefined or < 0, no delay will be imposed.
    delay: number;
    // The maximum delay to apply, in milliseconds. If > 0 and delay scales past
    // "maxDelay", it will be capped at "maxDelay".
    maxDelay: number | undefined;

    // delayScaling is the multiplier applied to "delay" in between retries. If
    // undefined or <= 1, DEFAULT_DELAY_SCALING will be used.
    delayScaling?: number;
  };

  /**
   * Generic exponential backoff retry delay generator.
   *
   * A RetryIterator is a specific configured instance of a Retry. Each call to
   * "next()" returns the next delay in the retry sequence.
   */
  export class RetryIterator {
    // Default scaling if no delay is specified.
    static readonly DEAFULT_DELAY_SCALING = 2;

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
        this.delayScaling = RetryIterator.DEAFULT_DELAY_SCALING;
      }
    }

    /**
     * @returns the next delay, in milliseconds. If there are no more retries,
     *      returns undefined.
     */
    next(): number|undefined {
      // Apply retries, if they have been enabled.
      if (this.retries !== undefined) {
        if (this.retries <= 0) {
          // No more retries remaining.
          return undefined;
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
  }
}
