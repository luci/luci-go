// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

// RegisterGlobalCallback registers a callback function that will be run once
// per minute on *one* instance of your Appengine app.
// You must specify the list of metrics that your callback affects - these
// metrics will be reset after flushing to ensure they are not sent by multiple
// instances.
// RegisterGlobalCallback should be called from an init() function in your
// module.
func RegisterGlobalCallback(f tsmon.Callback, metrics ...types.Metric) {
	RegisterGlobalCallbackIn(context.Background(), f, metrics...)
}

// RegisterGlobalCallbackIn is like RegisterGlobalCallback but registers in a
// given context.
func RegisterGlobalCallbackIn(c context.Context, f tsmon.Callback, metrics ...types.Metric) {
	if len(metrics) == 0 {
		panic("RegisterGlobalCallback called without any metrics")
	}

	state := tsmon.GetState(c)
	state.CallbacksMutex.Lock()
	defer state.CallbacksMutex.Unlock()

	state.GlobalCallbacks = append(state.GlobalCallbacks, tsmon.GlobalCallback{f, metrics})
}

func runGlobalCallbacks(c context.Context) {
	state := tsmon.GetState(c)
	state.CallbacksMutex.RLock()
	defer state.CallbacksMutex.RUnlock()

	for _, gcp := range state.GlobalCallbacks {
		gcp.Callback(c)
	}
}

func resetGlobalCallbackMetrics(c context.Context) {
	state := tsmon.GetState(c)
	state.CallbacksMutex.RLock()
	defer state.CallbacksMutex.RUnlock()

	for _, gcp := range state.GlobalCallbacks {
		for _, m := range gcp.Metrics {
			state.S.Reset(c, m)
		}
	}
}
