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
  export function sleepPromise(delay = 0, op?: Operation) {
    if (delay < 0) {
      delay = 0;
    }

    return new Promise<void>((resolve, reject) => {
      let handle: any = window.setTimeout(() => {
        handle = null;
        resolve();
      }, delay);

      if (op) {
        op.addCancelCallback(() => {
          if (handle) {
            window.clearTimeout(handle);
            reject(Operation.CANCELLED);
          }
        });
      }
    });
  }
}
