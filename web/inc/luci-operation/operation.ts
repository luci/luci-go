/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/

namespace luci {

  /**
   * An Operation wraps a Promise, adding methods to cancel that Promise.
   *
   * A cancelled Operation will completely halt, and will never call its
   * resolve or reject methods.
   *
   * Cancelling an operation also canceles all Operations chained to it,
   * allowing full-chain cancellation. Furthermore, Promises can register
   * cancellation callbacks with an Operation, allowing them to actively cancel
   * operations.
   */
  export class Operation {
    /**
     * CANCELLED is a sentinel error that is raised by assert if the Operation
     * has been cancelled.
     */
    static CANCELLED = new Error('Operation is cancelled');

    private _cancelled = false;
    private cancelCallbacks = new Array<() => void>();

    /**
     * Wraps a Promise, returning a Promise whose resolve and reject methods
     * will throw a CANCELLED error if this Operation has been cancelled.
     */
    wrap<T>(p: Promise<T>): Promise<T> {
      return new Promise((resolve, reject) => {
        p.then(
            result => {
              this.assert();
              resolve(result);
            },
            err => {
              this.assert();
              reject(err);
            });
      });
    }

    /** Cancelled returns true if this Operation has been cancelled. */
    get cancelled() {
      return this._cancelled;
    }

    /**
     * @throws Operation.CANCELLED if the Opertion has been cancelled.
     */
    assert() {
      if (this.cancelled) {
        throw Operation.CANCELLED;
      }
    }

    /**
     * Cancels the current Operation, marking it as cancelled and invoking any
     * registered cancellation callbacks.
     *
     * It is safe to call cancel more than once; however, additional calls will
     * have no effect.
     */
    cancel() {
      if (this.cancelled) {
        return;
      }

      // Mark that we are cancelled, and cancel our parent (and so on...).
      this._cancelled = true;
      this.cancelCallbacks.forEach(cb => asyncCall(cb));
    }

    /**
     * Registers a callback that will be invoked if this Operation is cancelled.
     *
     * @throws Operation.CANCELLED if the Operation has already been cancelled.
     */
    addCancelCallback(cb: () => void) {
      if (this.cancelled) {
        throw Operation.CANCELLED;
      }
      this.cancelCallbacks.push(cb);
    }

    /**
     * Removes all instances of the supplied callback from this Operation's
     * registered cancellation callbacks.
     */
    removeCancelCallback(cb: () => void) {
      while (true) {
        let index = this.cancelCallbacks.indexOf(cb);
        if (index < 0) {
          break;
        }
        this.cancelCallbacks.splice(index, 1);
      }
    }
  }

  /** Perform an asynchronous call. */
  function asyncCall(fn: () => void) {
    window.setTimeout(fn, 0);
  }
}
