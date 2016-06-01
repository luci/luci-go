// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package runtimestats exposes metrics related to the Go runtime.
//
// It exports the allocator statistics (go/mem/* metrics) and the current number
// of goroutines (go/goroutine/num).
//
// Should be invoked manually for now. Call Report(c) to populate the metrics
// prior tsmon Flush.
package runtimestats

import (
	"runtime"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/tsmon/metric"
)

var (
	// Some per-process memory allocator stats.
	// See https://golang.org/pkg/runtime/#MemStats

	MemAlloc       = metric.NewInt("go/mem/alloc", "Bytes allocated and not yet freed.")
	MemTotalAlloc  = metric.NewCounter("go/mem/total_alloc", "Bytes allocated (even if freed).")
	MemMallocs     = metric.NewCounter("go/mem/mallocs", "Number of mallocs.")
	MemFrees       = metric.NewCounter("go/mem/frees", "Number of frees.")
	MemNextGC      = metric.NewInt("go/mem/next_gc", "Next GC will happen when go/mem/alloc > this amount.")
	MemNumGC       = metric.NewCounter("go/mem/num_gc", "Number of garbage collections.")
	MemPauseTotal  = metric.NewCounter("go/mem/pause_total", "Total GC pause, in microseconds.")
	MemHeapSys     = metric.NewInt("go/mem/heap_sys", "Bytes obtained from system.")
	MemHeapIdle    = metric.NewInt("go/mem/heap_idle", "Bytes in idle spans.")
	MemHeapInuse   = metric.NewInt("go/mem/heap_in_use", "Bytes in non-idle span.")
	MemHeapObjects = metric.NewCounter("go/mem/heap_objects", "total number of allocated objects.")
	MemStackInuse  = metric.NewInt("go/mem/stack_in_use", "Bytes used by stack allocator.")
	MemStackSys    = metric.NewInt("go/mem/stack_in_sys", "Bytes allocated to stack allocator.")
	MemMSpanInuse  = metric.NewInt("go/mem/mspan_in_use", "Bytes used by mspan structures.")
	MemMSpanSys    = metric.NewInt("go/mem/mspan_in_sys", "Bytes allocated to mspan structures.")
	MemMCacheInuse = metric.NewInt("go/mem/mcache_in_use", "Bytes used by mcache structures.")
	MemMCacheSys   = metric.NewInt("go/mem/mcache_in_sys", "Bytes allocated to mcache structures.")

	// Other runtime stats.

	GoroutineNum = metric.NewInt("go/goroutine/num", "The number of goroutines that currently exist.")
)

// Report updates runtime stats metrics.
//
// Call it periodically (ideally right before flushing the metrics) to gather
// runtime stats metrics.
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
	MemHeapSys.Set(c, int64(stats.HeapSys))
	MemHeapIdle.Set(c, int64(stats.HeapIdle))
	MemHeapInuse.Set(c, int64(stats.HeapInuse))
	MemHeapObjects.Set(c, int64(stats.HeapObjects))
	MemStackInuse.Set(c, int64(stats.StackInuse))
	MemStackSys.Set(c, int64(stats.StackSys))
	MemMSpanInuse.Set(c, int64(stats.MSpanInuse))
	MemMSpanSys.Set(c, int64(stats.MSpanSys))
	MemMCacheInuse.Set(c, int64(stats.MCacheInuse))
	MemMCacheSys.Set(c, int64(stats.MCacheSys))

	GoroutineNum.Set(c, int64(runtime.NumGoroutine()))
}
