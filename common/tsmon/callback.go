// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"golang.org/x/net/context"
)

// Callback is a function that is run at metric collection time to set the
// values of one or more metrics.  A callback can be registered with
// RegisterCallback.
type Callback func(ctx context.Context)

// RegisterCallback registers a callback function that will be run at metric
// collection time to set the values of one or more metrics.  RegisterCallback
// should be called from an init() function in your module.
func RegisterCallback(f Callback) {
	RegisterCallbackIn(context.Background(), f)
}

// RegisterCallbackIn is like RegisterCallback but registers in a given context.
func RegisterCallbackIn(c context.Context, f Callback) {
	state := GetState(c)
	state.CallbacksMutex.Lock()
	defer state.CallbacksMutex.Unlock()

	state.Callbacks = append(state.Callbacks, f)
}

func runCallbacks(c context.Context) {
	state := GetState(c)
	state.CallbacksMutex.RLock()
	defer state.CallbacksMutex.RUnlock()

	for _, f := range state.Callbacks {
		f(c)
	}
}
