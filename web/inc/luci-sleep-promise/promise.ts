/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.
*/
//
///<reference path="../luci-operation/operation.ts" />

namespace luci {

  /**
   * Returns a Promise that resolves after a specified delay.
   *
   * An optional Operation may be supplied, in which case sleep Promise will be
   * automatically cancelled if the Operation is cancelled.
   */
  export function sleepPromise(delay = 0, op?: Operation): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      let t = new Timer(delay, resolve);

      if (op) {
        op.addCancelCallback(() => {
          if (t.cancel()) {
            reject(Operation.CANCELLED);
          }
        });
      }
    });
  }

  /**
   * Timer invokes a callback after a specified number of milliseconds.
   */
  export class Timer {
    private handle: number|undefined;

    constructor(delayMs: number, private callback: () => void) {
      if (delayMs < 0) {
        delayMs = 0;
      }
      this.handle = window.setTimeout(() => {
        this.handle = undefined;
        this.callback();
      }, delayMs);
    }

    /**
     * @return true if this Timer is currently active.
     */
    get active(): boolean {
      return (this.handle !== undefined);
    }

    /**
     * Cancels the Timer, preventing its callback from happening.
     *
     * If the Timer has already fired, or if the Timer has already been
     * cancelled, this will do nothing.
     *
     * @return true if the Timer was active.
     */
    cancel(): boolean {
      if (this.handle === undefined) {
        return false;
      }
      window.clearTimeout(this.handle);
      this.handle = undefined;
      return true;
    }
  }
}
