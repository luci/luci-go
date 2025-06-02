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

// Package profiling provides a tool to profile various aspects of the process.
package profiling

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	httpProf "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Profiler helps setup and manage profiling
type Profiler struct {
	// BindHTTP, if not empty, is the HTTP address to bind to.
	//
	// Can also be configured with "-profile-bind-http" flag.
	BindHTTP string

	// Dir, if set, is the path where profiling data will be written to.
	//
	// Can also be configured with "-profile-output-dir" flag.
	Dir string

	// ProfileCPU, if true, indicates that the profiler should profile the CPU.
	//
	// Requires Dir to be set, since it's where the profiler output is dumped.
	//
	// Can also be set with "-profile-cpu".
	ProfileCPU bool

	// ProfileTrace, if true, indicates that the profiler should enable
	// runtime tracing.
	//
	// Requires Dir to be set, since it's where the profiler output is dumped.
	//
	// Can also be set with "-profile-trace".
	ProfileTrace bool

	// ProfileHeap, if true, indicates that the profiler should profile heap
	// allocations.
	//
	// Requires Dir to be set, since it's where the profiler output is dumped.
	//
	// Can also be set with "-profile-heap".
	ProfileHeap bool

	// ProfileHeapFrequency, if set non-zero, instructs the profiler to
	// periodically dump heap profiler snapshots.
	//
	// Requires Dir to be set, since it's where the profiler output is dumped.
	//
	// Can also be set with "-profile-heap-frequency".
	ProfileHeapFrequency time.Duration

	// Logger, if not nil, will be used to log events and errors. If nil, no
	// logging will be used.
	Logger logging.Logger
	// Clock is the clock instance to use. If nil, the system clock will be used.
	Clock clock.Clock

	// listener is the active listener instance. It is set when Start is called.
	listener net.Listener

	// pathCounter is an atomic counter used to ensure non-conflicting paths.
	pathCounter uint32

	// profilingCPU is true if 'Start' successfully launched CPU profiling.
	profilingCPU bool

	// profilingTrace is true if 'Start' successfully launched runtime tracing.
	profilingTrace bool
}

// AddFlags adds command line flags to common Profiler fields.
func (p *Profiler) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&p.BindHTTP, "profile-bind-http", "",
		"If specified, run a runtime profiler HTTP server bound to this [address][:port].")
	fs.StringVar(&p.Dir, "profile-output-dir", "",
		"If specified, allow generation of profiling artifacts, which will be written here.")
	fs.BoolVar(&p.ProfileCPU, "profile-cpu", false, "If specified, enables CPU profiling.")
	fs.BoolVar(&p.ProfileTrace, "profile-trace", false, "If specified, enables runtime tracing.")
	fs.BoolVar(&p.ProfileHeap, "profile-heap", false, "If specified, enables heap profiling.")
	fs.DurationVar(&p.ProfileHeapFrequency, "profile-heap-frequency", 0, "If specified non-zero, enables periodic heap profiler snapshots dump.")
}

// Start starts the Profiler's configured operations.  On success, returns a
// function that can be called to shutdown the profiling server.
//
// Calling Stop is not necessary, but will enable end-of-operation profiling
// to be gathered.
func (p *Profiler) Start() error {
	if p.Dir == "" {
		if p.ProfileCPU {
			return errors.New("-profile-cpu requires -profile-output-dir to be set")
		}
		if p.ProfileTrace {
			return errors.New("-profile-trace requires -profile-output-dir to be set")
		}
		if p.ProfileHeap {
			return errors.New("-profile-heap requires -profile-output-dir to be set")
		}

		if p.ProfileHeapFrequency > 0 {
			return errors.New("-profile-heap-frequency requires -profile-output-dir to be set")
		}
	}

	if p.ProfileHeapFrequency < 0 {
		return errors.New("-profile-heap-frequency should be positive if set")
	}

	if p.ProfileHeapFrequency > 0 && !p.ProfileHeap {
		return errors.New("-profile-heap-frequency requires -profile-heap")
	}

	if p.ProfileCPU {
		out, err := os.Create(p.generateOutPath("cpu"))
		if err != nil {
			return errors.Fmt("failed to create CPU profile output file: %w", err)
		}
		pprof.StartCPUProfile(out)
		p.profilingCPU = true
	}

	if p.ProfileTrace {
		out, err := os.Create(p.generateOutPath("trace"))
		if err != nil {
			return errors.Fmt("failed to create runtime trace output file: %w", err)
		}
		trace.Start(out)
		p.profilingTrace = true
	}

	if p.ProfileHeapFrequency > 0 {
		go func() {
			for {
				time.Sleep(p.ProfileHeapFrequency)
				if err := p.dumpHeapProfile(); err != nil {
					p.getLogger().Errorf("Error dump heap profile: %v", err)
				}
			}
		}()
	}

	if p.BindHTTP != "" {
		if err := p.startHTTP(); err != nil {
			return errors.Fmt("failed to start HTTP server: %w", err)
		}
	}

	return nil
}

func (p *Profiler) startHTTP() error {
	// Register paths: https://golang.org/src/net/http/pprof/pprof.go
	router := http.NewServeMux()
	router.HandleFunc("/debug/pprof/", httpProf.Index)
	router.HandleFunc("/debug/pprof/cmdline", httpProf.Cmdline)
	router.HandleFunc("/debug/pprof/profile", httpProf.Profile)
	router.HandleFunc("/debug/pprof/symbol", httpProf.Symbol)
	router.HandleFunc("/debug/pprof/trace", httpProf.Trace)
	for _, p := range pprof.Profiles() {
		name := p.Name()
		router.Handle(fmt.Sprintf("/debug/pprof/%s", name), httpProf.Handler(name))
	}

	// Bind to our profiling port.
	l, err := net.Listen("tcp4", p.BindHTTP)
	if err != nil {
		return errors.Fmt("failed to bind to TCP4 address: %q: %w", p.BindHTTP, err)
	}

	server := http.Server{
		Handler: router,
	}
	go func() {
		if err := server.Serve(l); err != nil {
			p.getLogger().Errorf("Error serving profile HTTP: %s", err)
		}
	}()
	return nil
}

// Stop stops the Profiler's operations.
func (p *Profiler) Stop() {
	if p.profilingCPU {
		pprof.StopCPUProfile()
		p.profilingCPU = false
	}

	if p.profilingTrace {
		trace.Stop()
		p.profilingTrace = false
	}

	if p.listener != nil {
		if err := p.listener.Close(); err != nil {
			p.getLogger().Warningf("Failed to stop profile HTTP server: %s", err)
		}
		p.listener = nil
	}

	// Take one final snapshot.
	p.DumpSnapshot()
}

// DumpSnapshot dumps a profile snapshot to the configured output directory. If
// no output directory is configured, nothing will happen.
func (p *Profiler) DumpSnapshot() error {
	if p.Dir == "" {
		return nil
	}

	if p.ProfileHeap {
		if err := p.dumpHeapProfile(); err != nil {
			return errors.Fmt("failed to dump heap profile: %w", err)
		}
	}

	return nil
}

func (p *Profiler) dumpHeapProfile() error {
	fd, err := os.Create(p.generateOutPath("memory"))
	if err != nil {
		return errors.Fmt("failed to create output file: %w", err)
	}
	defer fd.Close()

	// Get up-to-date statistics.
	runtime.GC()
	if err := pprof.WriteHeapProfile(fd); err != nil {
		return errors.Fmt("failed to write heap profile: %w", err)
	}
	return nil
}

func (p *Profiler) generateOutPath(base string) string {
	clk := p.Clock
	if clk == nil {
		clk = clock.GetSystemClock()
	}
	now := clk.Now()
	counter := atomic.AddUint32(&p.pathCounter, 1) - 1
	return filepath.Join(p.Dir, fmt.Sprintf("%s_%d_%d.prof", base, now.Unix(), counter))
}

func (p *Profiler) getLogger() logging.Logger {
	if p.Logger != nil {
		return p.Logger
	}
	return logging.Null
}
