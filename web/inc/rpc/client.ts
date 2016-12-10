/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

import * as luci_sleep_promise from "luci-sleep-promise/promise";

export namespace luci_rpc {

  interface PolymerClient {
    service: string;
    method: string;
    request: any;

    call(): {
      completes: Promise<any>,
    };
  }

  export class Client {
    transientRetry: Retry = new Retry(new RetryIterator(10, 500, null));
    private pc: PolymerClient;

    constructor(pc: any) {
      this.pc = pc as PolymerClient;
    }

    /** Call invokes the specified service's method, returning a Promise. */
    call<T, R>(service: string, method: string, request?: T): Promise<R> {
      this.pc.service = service;
      this.pc.method = method;
      this.pc.request = request;

      let transientRetry = this.transientRetry.iterator();
      let doCall = (): Promise<R> => {
        let callPromise: Promise<R> = this.pc.call().completes;
        return callPromise.then( (resp: any) => {
          return resp.response;
        }).catch( (err: Error) => {
          // Is this a transient error?
          if ( isTransientError(err) ) {
            let delay = transientRetry.next();
            if ( delay ) {
              console.warn(
                `Transient error calling ${service}.${method} with params:`,
                request, `:`, err, `; retrying after ${delay}ms.`);
              return luci_sleep_promise.sleep(delay).then( () => {
                return doCall();
              });
            }
          }

          // Non-transient, throw the error.
          throw err;
        });
      };
      return doCall();
    }
  }

  /** gRPC Codes */
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

  export class GrpcError extends Error {
    constructor(readonly code: Code, readonly description?: string) {
      super("code = " + code + ", desc = " + description);
    }

    /**
     * Converts the supplied Error into a GrpcError if its name is
     * "GrpcError". This merges between the non-Typescript RPC code and this
     * error type.
     */
    static convert(err: Error): GrpcError | null {
      if ( err.name === 'GrpcError' ) {
        let aerr = err as GrpcError;
        return new GrpcError(aerr.code, aerr.description);
      }
      return null;
    }

    /** Returns true if the error is considered transient. */
    get transient(): boolean {
      switch ( this.code ) {
      case Code.INTERNAL:
      case Code.UNAVAILABLE:
      case Code.RESOURCE_EXHAUSTED:
        return true;

      default:
        return false;
      }
    }
  }

  export class HttpError extends Error {
    constructor(readonly code: Code, readonly description?: string) {
      super("code = " + code + ", desc = " + description);
    }

    /**
     * Converts the supplied Error into a HttpError if its name is
     * "HttpError". This merges between the non-Typescript RPC code and this
     * error type.
     */
    static convert(err: Error): HttpError | null {
      if ( err.name === 'HttpError' ) {
        let aerr = err as HttpError;
        return new HttpError(aerr.code, aerr.description);
      }
      return null;
    }

    /** Returns true if the error is considered transient. */
    get transient(): boolean { return ( this.code >= 500 ); }
  }

  export function isTransientError(err: Error): boolean {
    let grpc = GrpcError.convert(err);
    if ( grpc ) {
      return grpc.transient;
    }

    // Is this an HTTP Error?
    let http = HttpError.convert(err);
    if ( http ) {
      return http.transient;
    }

    // Unknown error.
    return false;
  }

  export class RetryIterator {
    private retries: number | null;
    private delay: number;
    private maxDelay: number | null;

    constructor(retries: number | null, delay: number,
                maxDelay: number | null) {

      this.retries = retries;
      this.delay = delay;
      this.maxDelay = maxDelay;
    }

    clone(): RetryIterator {
      return new RetryIterator(this.retries, this.delay, this.maxDelay);
    }

    next(): number | null {
      if ( this.retries !== null ) {
        if ( this.retries <= 0 ) {
          // No more retries remaining.
          return null;
        }
        this.retries--;
      }

      let delay = this.delay;
      this.delay *= 2;
      if ( this.maxDelay !== null && this.delay > this.maxDelay ) {
        this.delay = this.maxDelay;
      }
      return delay;
    }
  }

  export class Retry {
    private base: RetryIterator;

    constructor(base: RetryIterator) {
      this.base = base;
    }

    iterator(): RetryIterator {
      return this.base.clone();
    }
  }

}
