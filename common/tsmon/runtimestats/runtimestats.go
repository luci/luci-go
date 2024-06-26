// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package runtimestats exposes metrics related to the Go runtime.
//
// It exports the allocator statistics (go/mem/* metrics) and the current number
// of goroutines (go/goroutine/num).
//
// Should be invoked manually for now. Call Report(c) to populate the metrics
// prior tsmon Flush.
package runtimestats

import (
	"context"
	"runtime"

	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	// Some per-process memory allocator stats.
	// See https://golang.org/pkg/runtime/#MemStats

	MemAlloc       = metric.NewInt("go/mem/alloc", "Bytes allocated and not yet freed.", &types.MetricMetadata{types.Bytes})
	MemTotalAlloc  = metric.NewCounter("go/mem/total_alloc", "Bytes allocated (even if freed).", &types.MetricMetadata{types.Bytes})
	MemMallocs     = metric.NewCounter("go/mem/mallocs", "Number of mallocs.", nil)
	MemFrees       = metric.NewCounter("go/mem/frees", "Number of frees.", nil)
	MemNextGC      = metric.NewInt("go/mem/next_gc", "Next GC will happen when go/mem/alloc > this amount.", nil)
	MemNumGC       = metric.NewCounter("go/mem/num_gc", "Number of garbage collections.", nil)
	MemPauseTotal  = metric.NewCounter("go/mem/pause_total", "Total GC pause, in microseconds.", nil)
	MemHeapSys     = metric.NewInt("go/mem/heap_sys", "Bytes obtained from system.", &types.MetricMetadata{types.Bytes})
	MemHeapIdle    = metric.NewInt("go/mem/heap_idle", "Bytes in idle spans.", &types.MetricMetadata{types.Bytes})
	MemHeapInuse   = metric.NewInt("go/mem/heap_in_use", "Bytes in non-idle span.", &types.MetricMetadata{types.Bytes})
	MemHeapObjects = metric.NewInt("go/mem/heap_objects", "Total number of allocated objects.", nil)
	MemStackInuse  = metric.NewInt("go/mem/stack_in_use", "Bytes used by stack allocator.", &types.MetricMetadata{types.Bytes})
	MemStackSys    = metric.NewInt("go/mem/stack_in_sys", "Bytes allocated to stack allocator.", &types.MetricMetadata{types.Bytes})
	MemMSpanInuse  = metric.NewInt("go/mem/mspan_in_use", "Bytes used by mspan structures.", &types.MetricMetadata{types.Bytes})
	MemMSpanSys    = metric.NewInt("go/mem/mspan_in_sys", "Bytes allocated to mspan structures.", &types.MetricMetadata{types.Bytes})
	MemMCacheInuse = metric.NewInt("go/mem/mcache_in_use", "Bytes used by mcache structures.", &types.MetricMetadata{types.Bytes})
	MemMCacheSys   = metric.NewInt("go/mem/mcache_in_sys", "Bytes allocated to mcache structures.", &types.MetricMetadata{types.Bytes})

	// Other runtime stats.

	GoroutineNum = metric.NewInt("go/goroutine/num", "The number of goroutines that currently exist.", nil)
)

// Report updates runtime stats metrics.
//
// Call it periodically (ideally right before flushing the metrics) to gather
// runtime stats metrics.
func Report(ctx context.Context) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	MemAlloc.Set(ctx, int64(stats.Alloc))
	MemTotalAlloc.Set(ctx, int64(stats.TotalAlloc))
	MemMallocs.Set(ctx, int64(stats.Mallocs))
	MemFrees.Set(ctx, int64(stats.Frees))
	MemNextGC.Set(ctx, int64(stats.NextGC))
	MemNumGC.Set(ctx, int64(stats.NumGC))
	MemPauseTotal.Set(ctx, int64(stats.PauseTotalNs/1000))
	MemHeapSys.Set(ctx, int64(stats.HeapSys))
	MemHeapIdle.Set(ctx, int64(stats.HeapIdle))
	MemHeapInuse.Set(ctx, int64(stats.HeapInuse))
	MemHeapObjects.Set(ctx, int64(stats.HeapObjects))
	MemStackInuse.Set(ctx, int64(stats.StackInuse))
	MemStackSys.Set(ctx, int64(stats.StackSys))
	MemMSpanInuse.Set(ctx, int64(stats.MSpanInuse))
	MemMSpanSys.Set(ctx, int64(stats.MSpanSys))
	MemMCacheInuse.Set(ctx, int64(stats.MCacheInuse))
	MemMCacheSys.Set(ctx, int64(stats.MCacheSys))

	GoroutineNum.Set(ctx, int64(runtime.NumGoroutine()))
}
