// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
	"sync/atomic"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
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

	// Logger, if not nil, will be used to log events and errors. If nil, no
	// logging will be used.
	Logger logging.Logger
	// Clock is the clock instance to use. If nil, the system clock will be used.
	Clock clock.Clock

	// listener is the active listener instance. It is set when Start is called.
	listener net.Listener

	// pathCounter is an atomic counter used to ensure non-conflicting paths.
	pathCounter uint32
}

// AddFlags adds command line flags to common Profiler fields.
func (p *Profiler) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&p.BindHTTP, "profile-bind-http", "",
		"If specified, run a runtime profiler HTTP server bound to this [address][:port].")
	fs.StringVar(&p.Dir, "profile-output-dir", "",
		"If specified, allow generation of profiling artifacts, which will be written here.")
}

// Start starts the Profiler's configured operations.  On success, returns a
// function that can be called to shutdown the profiling server.
//
// Calling Stop is not necessary, but will enable end-of-operation profiling
// to be gathered.
func (p *Profiler) Start() error {
	// If we have an output directory, start our CPU profiling.
	if p.BindHTTP != "" {
		if err := p.startHTTP(); err != nil {
			return errors.Annotate(err).Reason("failed to start HTTP server").Err()
		}
	}
	return nil
}

func (p *Profiler) startHTTP() error {
	// Register paths: https://golang.org/src/net/http/pprof/pprof.go
	router := httprouter.New()
	router.HandlerFunc("GET", "/debug/pprof/", httpProf.Index)
	router.HandlerFunc("GET", "/debug/pprof/cmdline", httpProf.Cmdline)
	router.HandlerFunc("GET", "/debug/pprof/profile", httpProf.Profile)
	router.HandlerFunc("GET", "/debug/pprof/symbol", httpProf.Symbol)
	router.HandlerFunc("GET", "/debug/pprof/trace", httpProf.Trace)
	for _, p := range pprof.Profiles() {
		name := p.Name()
		router.Handler("GET", fmt.Sprintf("/debug/pprof/%s", name), httpProf.Handler(name))
	}

	// Bind to our profiling port.
	l, err := net.Listen("tcp4", p.BindHTTP)
	if err != nil {
		return errors.Annotate(err).Reason("failed to bind to TCP4 address: %(addr)q").
			D("addr", p.BindHTTP).Err()
	}

	server := http.Server{
		Handler: http.HandlerFunc(router.ServeHTTP),
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

	if err := p.dumpHeapProfile(); err != nil {
		return errors.Annotate(err).Reason("failed to dump heap profile").Err()
	}
	return nil
}

func (p *Profiler) dumpHeapProfile() error {
	fd, err := os.Create(p.generateOutPath("memory"))
	if err != nil {
		return errors.Annotate(err).Reason("failed to create output file").Err()
	}
	defer fd.Close()

	// Get up-to-date statistics.
	runtime.GC()
	if err := pprof.WriteHeapProfile(fd); err != nil {
		return errors.Annotate(err).Reason("failed to write heap profile").Err()
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
