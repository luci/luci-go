// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package memstats exposes metrics related to the memory allocator.
//
// Invoked completely manually for now. Call Report(c) to populate the metrics
// based on what runtime.ReadMemStats returns.
package memstats

import (
	"runtime"

	"github.com/luci/luci-go/common/tsmon/metric"
	"golang.org/x/net/context"
)

var (
	// Some per-process memory allocator stats.
	// See https://golang.org/pkg/runtime/#MemStats

	MemAlloc      = metric.NewInt("go/mem/alloc", "Bytes allocated and not yet freed.")
	MemTotalAlloc = metric.NewCounter("go/mem/total_alloc", "Bytes allocated (even if freed).")
	MemMallocs    = metric.NewCounter("go/mem/mallocs", "Number of mallocs.")
	MemFrees      = metric.NewCounter("go/mem/frees", "Number of frees.")
	MemNextGC     = metric.NewInt("go/mem/next_gc", "Next GC will happen when go/mem/alloc > this amount.")
	MemNumGC      = metric.NewCounter("go/mem/num_gc", "Number of garbage collections.")
	MemPauseTotal = metric.NewCounter("go/mem/pause_total", "Total GC pause, in microseconds.")
)

// Report update mem stats metrics using runtime.ReadMemStats.
//
// Call it periodically (ideally right before flushing the metrics) to gather
// mem stats metrics.
func Report(c context.Context) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	MemAlloc.Set(c, int64(stats.Alloc))
	MemTotalAlloc.Set(c, int64(stats.TotalAlloc))
	MemMallocs.Set(c, int64(stats.Mallocs))
	MemFrees.Set(c, int64(stats.Frees))
	MemNextGC.Set(c, int64(stats.NextGC))
	MemNumGC.Set(c, int64(stats.NumGC))
	MemPauseTotal.Set(c, int64(stats.PauseTotalNs/1000))
}
