// Copyright 2020 The LUCI Authors.
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

package host

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/plugin"
	"go.chromium.org/luci/cipd/client/cipd/plugin/plugins/admission"
	"go.chromium.org/luci/cipd/client/cipd/plugin/protocol"
	"go.chromium.org/luci/cipd/common"
)

// ErrAborted is returned by CheckAdmission promise when the plugin terminates.
var ErrAborted = errors.Reason("the admission plugin is terminating").Err()

// AdmissionPlugin launches and communicates with an admission plugin.
//
// It is instantiated by the CIPD client if it detects there's an admission
// plugin configured.
//
// Implements plugin.AdmissionPlugin interface.
type AdmissionPlugin struct {
	ctx             context.Context // for logging from the plugin
	host            *Host           // the Host to run the plugin in
	args            []string        // plugin's command line
	salt            int             // randomization for generated admission IDs
	protocolVersion int32           // the expected protocol version

	timeout   time.Duration // how long to wait for the ListAdmissions RPC
	connects  int32         // incremented in ListAdmissions RPC
	connected chan struct{} // closed in the first ListAdmissions RPC

	wg        sync.WaitGroup      // waits for p.launchPlugin to finish
	m         sync.Mutex          // protects everything below
	err       error               // if non-nil, denies all CheckAdmission calls
	launching bool                // true if we attempted to launch the plugin
	proc      *PluginProcess      // the running process, if started successfully
	closing   chan struct{}       // closed in Close
	closed    bool                // true if Close was called
	checks    map[string]*Promise // pending and finished checks
	pending   chan *Promise       // pending checks
}

// Promise is a pending or finished result of an admission check.
//
// Implements plugin.Promise interface.
type Promise struct {
	check    *protocol.Admission
	resolves int32         // how many times resolve(...) was called
	done     chan struct{} // closed in `resolve`
	err      error         // the result of the check (usually a gRPC status)
}

// newPromise constructs a new unresolved promise.
func newPromise(check *protocol.Admission) *Promise {
	return &Promise{
		check: check,
		done:  make(chan struct{}),
	}
}

// Wait blocks until the promise is fulfilled or the context expires.
//
// Returns nil if the admission check passed.
func (p *Promise) Wait(ctx context.Context) error {
	select {
	case <-p.done:
		return p.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// resolve records the check result and unblocks all Waits.
//
// Does nothing if the promise is already resolved.
func (p *Promise) resolve(err error) *Promise {
	if atomic.AddInt32(&p.resolves, 1) == 1 {
		p.err = err
		close(p.done)
	}
	return p
}

// resolved checks if the promise is already resolved.
func (p *Promise) resolved() bool {
	return atomic.LoadInt32(&p.resolves) != 0
}

// NewAdmissionPlugin returns a host-side representation of an admission plugin.
//
// The returned *AdmissionPlugin can be used right away to enqueue admission
// checks. The plugin subprocess will lazily be started on the first
// CheckAdmission call. All enqueued checks will eventually be processed by
// the plugin or rejected if the plugin fails to start.
//
// The context is used for logging from the plugin.
func NewAdmissionPlugin(ctx context.Context, host *Host, args []string) *AdmissionPlugin {
	return &AdmissionPlugin{
		ctx:             ctx,
		host:            host,
		args:            append([]string(nil), args...),
		salt:            os.Getpid(), // note: predictability is fine
		protocolVersion: admission.ProtocolVersion,
		timeout:         30 * time.Second, // see launchPlugin
		connected:       make(chan struct{}),
		closing:         make(chan struct{}),
		checks:          map[string]*Promise{},
		pending:         make(chan *Promise, 1000000), // ~infinite
	}
}

// Executable is a path to this plugin's executable.
func (p *AdmissionPlugin) Executable() string {
	return p.args[0]
}

// Close terminates the plugin (if it was running) and aborts all pending
// checks.
//
// Tries to gracefully terminate the plugin, killing it with SIGKILL on the
// context timeout or after 5 sec.
//
// Note that calling Close is not necessary if the plugin host itself
// terminates. The plugin subprocess will be terminated by the host in this
// case.
func (p *AdmissionPlugin) Close(ctx context.Context) {
	defer p.wg.Wait() // don't leak launchPlugin goroutine past Plugin's lifetime

	p.m.Lock()
	if !p.closed {
		p.closed = true
		p.rejectAllLocked(ErrAborted) // set p.err, abort all pending checks
		close(p.closing)              // notify launchPlugin (if running) to abort
	}
	proc := p.proc
	p.proc = nil
	p.m.Unlock()

	if proc != nil {
		proc.Terminate(ctx)
	}
}

// CheckAdmission enqueues an admission check to be performed by the plugin.
//
// The plugin will be asked if it's OK to deploy a package with the given pin
// hosted on the CIPD service used by the running CIPD client.
//
// Returns a promise which is resolved when the result is available. If such
// check is already pending (or has been done before), returns an existing
// (perhaps already resolved) promise.
func (p *AdmissionPlugin) CheckAdmission(pin common.Pin) plugin.Promise {
	admission, err := p.makeAdmission(pin)
	if err != nil {
		return newPromise(nil).resolve(err)
	}

	p.m.Lock()
	defer p.m.Unlock()

	// Reuse an existing promise (either pending or finished) if available.
	if existing := p.checks[admission.AdmissionId]; existing != nil {
		return existing
	}

	// If already closed or broken, fail the check right away.
	if p.err != nil {
		return newPromise(nil).resolve(p.err)
	}

	// If we haven't tried to launch the plugin process yet, do it now.
	if !p.launching {
		p.launching = true
		p.wg.Add(1)
		go p.launchPlugin()
	}

	// Enqueue this request for processing when the plugin process is up.
	promise := newPromise(admission)
	p.checks[admission.AdmissionId] = promise
	p.pending <- promise
	return promise
}

// ClearCache drops all resolved promises to free up some memory.
func (p *AdmissionPlugin) ClearCache() {
	p.m.Lock()
	defer p.m.Unlock()
	for id, promise := range p.checks {
		if promise.resolved() {
			delete(p.checks, id)
		}
	}
}

// makeAdmission prepares *protocol.Admission, generating its ID.
//
// It hashes the request to make sure plugins do not rely on a particular format
// of the ID. It also randomizes it with some salt, to make sure plugins do not
// try to use it as a key in some persistent cache. Admission IDs are ephemeral.
func (p *AdmissionPlugin) makeAdmission(pin common.Pin) (*protocol.Admission, error) {
	admission := &protocol.Admission{
		ServiceUrl: p.host.Config().ServiceURL,
		Package:    pin.PackageName,
		Instance:   common.InstanceIDToObjectRef(pin.InstanceID),
	}

	// Binary proto serialization within a single process is stable. We can use it
	// to derive an ID.
	blob, err := proto.Marshal(admission)
	if err != nil {
		return nil, errors.Annotate(err, "failed to serialize Admission").Err()
	}

	// Mix in the salt to randomize admission IDs across CIPD client processes.
	h := sha256.New()
	fmt.Fprintf(h, "%d\n", p.salt)
	h.Write(blob)

	admission.AdmissionId = base64.RawStdEncoding.EncodeToString(h.Sum(nil))
	return admission, nil
}

// rejectAllLocked rejects all pending and future admission checks.
//
// Must be called with p.m locked.
func (p *AdmissionPlugin) rejectAllLocked(err error) {
	if p.err == nil {
		p.err = err
		for _, promise := range p.checks {
			promise.resolve(p.err)
		}
		close(p.pending)
		for range p.pending {
		}
	}
}

// launchPlugin launches the plugin subprocess and waits for it to connect.
//
// It is called from a background goroutine on a first CheckAdmission call.
func (p *AdmissionPlugin) launchPlugin() {
	defer p.wg.Done()

	proc, err := p.host.LaunchPlugin(p.ctx, p.args, &Controller{
		Admissions: &admissionsServer{plugin: p},
	})

	if err == nil {
		ctx, cancel := context.WithTimeout(p.ctx, p.timeout)
		defer cancel()
		select {
		case <-p.connected:
			// The plugin called ListAdmissions and is listening for requests now or
			// we asked it to go away due to incompatible protocol version (in which
			// case p.err is already set).
		case <-p.closing:
			// Already closing, p.err is not nil and will be handled below.
		case <-proc.Done():
			err = errors.WrapIf(proc.Err(), "the admission plugin terminated before making ListAdmissions RPC")
		case <-ctx.Done():
			err = errors.WrapIf(ctx.Err(), "while waiting for ListAdmissions RPC")
		}
	}

	p.m.Lock()
	switch {
	case p.err != nil:
		// We are closing or broken. The plugin process is no longer needed.
		err = p.err
	case err != nil:
		// The plugin failed to start, move us into the "broken" state.
		logging.Warningf(p.ctx, "The admission plugin failed to start: %s", err)
		p.rejectAllLocked(err)
	default:
		// The plugin has started successfully and some processing has begun!
		p.proc = proc
	}
	p.m.Unlock()

	// Kill the plugin if it is no longer needed.
	if err != nil && proc != nil {
		proc.Terminate(p.ctx)
	}
}

// onConnected is called when the plugin makes ListAdmissions RPC.
func (p *AdmissionPlugin) onConnected(req *protocol.ListAdmissionsRequest) error {
	// At most one ListAdmissions call per plugin's life cycle is allowed, since
	// we use its completion as a signal that the plugin has disconnected (e.g.
	// unexpectedly died). There's no sudden unexpected disconnects on localhost.
	if atomic.AddInt32(&p.connects, 1) != 1 {
		return status.Errorf(codes.FailedPrecondition, "already called ListAdmissions")
	}
	logging.Debugf(p.ctx, "Using deployment admission plugin %q", req.PluginVersion)

	var err error
	if req.ProtocolVersion != p.protocolVersion {
		logging.Errorf(p.ctx, "Unknown admission plugin protocol %d: expecting %d", req.ProtocolVersion, p.protocolVersion)
		err = status.Errorf(codes.FailedPrecondition, "unknown protocol version %d: expecting %d", req.ProtocolVersion, p.protocolVersion)
		p.m.Lock()
		p.rejectAllLocked(err)
		p.m.Unlock()
	}

	close(p.connected)
	return err
}

// onDisconnected is called when the plugin aborts ListAdmissions RPC.
//
// Note that it can potentially happen even before launchPlugin completes, if
// the plugin crashed particularly fast.
func (p *AdmissionPlugin) onDisconnected() {
	p.m.Lock()
	defer p.m.Unlock()

	err := ErrAborted

	// Quickly poll for the process status: if it crashed hard, we'd like to know.
	// If 50 ms is not enough for it to terminate after the disconnect, no big
	// deal, a generic error message in ErrAborted will suffice too.
	if p.proc != nil {
		select {
		case <-time.After(50 * time.Millisecond):
		case <-p.proc.Done():
			if p.proc.Err() != ErrTerminated {
				logging.Warningf(p.ctx, "The admission plugin has crashed: %s", p.proc.Err())
			}
			err = errors.WrapIf(p.proc.Err(), "the admission plugin terminated")
		}
	}

	p.rejectAllLocked(err)
}

// dequeue blocks until there's an unprocessed admission request available.
//
// Respects context's expiration.
func (p *AdmissionPlugin) dequeue(ctx context.Context) (*protocol.Admission, error) {
	for {
		select {
		case promise := <-p.pending:
			// There are two concurrent termination paths once the host decides to
			// stop the plugin: (1) it replies with codes.Aborted below, and (2) it
			// closes the plugin's stdin.
			//
			// (2) can win the race, which results in the plugin canceling
			// ListAdmissions on its own before receiving codes.Aborted. It manifests
			// as 'ctx' here being canceled.
			//
			// The termination path that uses stdin is more general and works for
			// any kind of a plugin. The path (1) exists because we need to react to
			// p.pending closing somehow. There's probably a way to get rid of it, but
			// it'll make the code more complicated.
			if promise == nil {
				return nil, status.Errorf(codes.Aborted, "terminating")
			}
			if promise.resolved() {
				// Likely we are already closing and the promise was resolved in
				// rejectAllLocked. If so, keep draining the channel until it returns
				// nil.
				continue
			}
			return promise.check, nil
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
	}
}

// resolve is called when an admission request is resolved by the plugin.
func (p *AdmissionPlugin) resolve(id string, err error) {
	p.m.Lock()
	promise := p.checks[id]
	p.m.Unlock()
	if promise != nil {
		promise.resolve(err)
	}
}

////////////////////////////////////////////////////////////////////////////////

// admissionsServer receives RPCs from some single admission plugin.
type admissionsServer struct {
	protocol.UnimplementedAdmissionsServer
	plugin *AdmissionPlugin
}

func (s *admissionsServer) ListAdmissions(req *protocol.ListAdmissionsRequest, stream protocol.Admissions_ListAdmissionsServer) error {
	if err := s.plugin.onConnected(req); err != nil {
		return err
	}
	defer s.plugin.onDisconnected()

	for {
		admission, err := s.plugin.dequeue(stream.Context())
		if err != nil {
			return err
		}
		if err := stream.Send(admission); err != nil {
			return err
		}
	}
}

func (s *admissionsServer) ResolveAdmission(ctx context.Context, req *protocol.ResolveAdmissionRequest) (*emptypb.Empty, error) {
	s.plugin.resolve(req.AdmissionId, status.ErrorProto(req.Status))
	return &emptypb.Empty{}, nil
}
