// Copyright 2015 The LUCI Authors.
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

package butler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bundler"
	"go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
)

const (
	// DefaultMaxBufferAge is the default amount of time that a log entry may
	// be buffered before being dispatched.
	DefaultMaxBufferAge = time.Duration(5 * time.Second)

	// streamBufferSize is the maximum amount of stream data to buffer in memory.
	streamBufferSize = 1024 * 1024 * 5
)

// Config is the set of Butler configuration parameters.
type Config struct {
	// Output is the output instance to use for log dispatch.
	Output output.Output

	// GlobalTags are a set of global log stream tags to apply to individual
	// streams on registration. Individual stream tags will override tags with
	// the same key.
	GlobalTags streamproto.TagMap

	// BufferLogs, if true, instructs the butler to buffer collected log data
	// before sending it to Output.
	BufferLogs bool

	// If buffering logs, this is the maximum amount of time that a log will
	// be buffered before being marked for dispatch. If this is zero,
	// DefaultMaxBufferAge will be used.
	MaxBufferAge time.Duration
}

// Validate validates that the configuration is sufficient to instantiate a
// Butler instance.
func (c *Config) Validate() error {
	if c.Output == nil {
		return errors.New("butler: an Output must be supplied")
	}
	return nil
}

type registeredCallback struct {
	cb   StreamRegistrationCallback
	wrap bool
}

// Butler is the Butler application structure. The Butler runs until closed.
// During operation, it acts as a service manager and data router, routing:
// - Messages from Streams to the attached Output.
// - Streams from a StreamServer to the Stream list (AddStream).
type Butler struct {
	c   *Config
	ctx context.Context

	// streamRegistrationCallbacks is the list of callbacks the Bundler
	streamRegistrationCallbacksMu sync.RWMutex
	streamRegistrationCallbacks   []registeredCallback
	streamCallbacksMu             sync.RWMutex
	streamCallbacks               map[string]StreamChunkCallback

	// bundler is the Bundler instance.
	bundler *bundler.Bundler
	// bundlerDrainedC is a signal channel that is closed when the Bundler has
	// been drained.
	bundlerDrainedC chan struct{}

	// activateC is closed when Activate() is called.
	activateC chan struct{}
	// activateOnce ensures we close activeC exactly once.
	activateOnce sync.Once

	// streamsFinishedC is a signal channel that will be closed when the stream
	// monitor finishes its managed stream set.
	streamsFinishedC chan struct{}

	// WaitGroup to manage running StreamServers.
	streamServerWG sync.WaitGroup
	// Channel to signal StreamServers to stop.
	streamServerStopC chan struct{}

	streamC chan *stream

	streams *streamTracker

	// shutdownMu is a mutex to protect shutdown parameters.
	shutdownMu sync.Mutex
	// isShutdown is true if the Butler been shut down.
	isShutdown bool
	// runErr is the error returned by Run.
	runErr error
	// streamStopC is a stop signal channel for stream. This will cause streams
	// to prematurely terminate (before EOF) on shutdown.
	streamStopC chan struct{}
}

// New instantiates a new Butler instance and starts its processing.
func New(ctx context.Context, config Config) (*Butler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	b := &Butler{
		c:   &config,
		ctx: ctx,

		streamCallbacks: map[string]StreamChunkCallback{},

		bundlerDrainedC: make(chan struct{}),

		streamsFinishedC: make(chan struct{}),

		streams: newStreamTracker(),

		activateC:         make(chan struct{}),
		streamC:           make(chan *stream),
		streamServerStopC: make(chan struct{}),
		streamStopC:       make(chan struct{}),
	}

	bc := bundler.Config{
		Clock:            clock.Get(ctx),
		MaxBufferedBytes: streamBufferSize,
		MaxBundleSize:    config.Output.MaxSize(),
	}
	if config.BufferLogs {
		bc.MaxBufferDelay = config.MaxBufferAge
		if bc.MaxBufferDelay <= 0 {
			bc.MaxBufferDelay = DefaultMaxBufferAge
		}
	}
	b.bundler = bundler.New(bc)

	// Load bundles from our Bundler into the queue.
	go func() {
		defer close(b.bundlerDrainedC)

		numWorkers := b.c.Output.MaxSendBundles()
		if numWorkers <= 0 {
			numWorkers = 1
		}

		parallel.Ignore(parallel.Run(numWorkers, func(workC chan<- func() error) {
			// Read bundles until the bundler is drained.
			for {
				bundle := b.bundler.Next()
				if bundle == nil {
					return
				}

				b.runCallbacks(bundle)

				workC <- func() error {
					b.c.Output.SendBundle(bundle)
					return nil
				}
			}
		}))
	}()

	// Run our primary stream monitor until the Butler instance is activated.
	go func() {
		defer close(b.streamsFinishedC)
		b.runStreams(b.activateC)
	}()

	// Shutdown our Butler if our Context is cancelled.
	go func() {
		select {
		case <-b.streamStopC:
			break
		case <-b.ctx.Done():
			log.WithError(b.ctx.Err()).Warningf(b.ctx, "Butler Context was cancelled. Initiating shutdown.")
			b.shutdown(b.ctx.Err())
		}
	}()

	return b, nil
}

// Wait blocks until the Butler instance has completed, returning with the
// Butler's return code.
func (b *Butler) Wait() error {
	// Run until our stream monitor shuts down, meaning all streams have finished.
	//
	// A race can exist here when a stream server may add new streams after we've
	// drained our streams, but before we've shut them down. Since "all streams
	// are done" is the edge that we use to begin shutdown, we can't simply tell
	// stream servers to stop in advance.
	//
	// We manage this race as follows:
	// 1) Wait until our stream monitor finishes. This will happen when there's a
	//    point that no streams are running.
	// 2) Start a new stream monitor to handle (3).
	// 3) Initiate stream server shutdown, wait until they have all finished.
	// 4) Wait until the stream monitor in (2) has finished.
	log.Debugf(b.ctx, "Waiting for Butler primary stream monitor to finish...")
	<-b.streamsFinishedC
	log.Debugf(b.ctx, "Butler streams have finished.")

	log.Debugf(b.ctx, "Shutting down stream servers, starting residual stream monitor.")

	auxStreamsFinishedC := make(chan struct{})
	auxActivateC := make(chan struct{})
	go func() {
		defer close(auxStreamsFinishedC)
		b.runStreams(auxActivateC)
	}()

	close(b.streamServerStopC)
	b.streamServerWG.Wait()
	log.Debugf(b.ctx, "Stream servers have shut down.")

	log.Debugf(b.ctx, "Waiting for residual streams to finish...")
	close(b.streamC)
	close(auxActivateC)
	<-auxStreamsFinishedC
	log.Debugf(b.ctx, "Residual streams have finished.")

	log.Debugf(b.ctx, "Waiting for bundler to flush.")
	b.bundler.CloseAndFlush()
	log.Debugf(b.ctx, "Bundler has flushed.")

	log.Debugf(b.ctx, "Waiting for output queue to shut down.")
	<-b.bundlerDrainedC
	log.Debugf(b.ctx, "Output queue has shut down.")

	log.Fields{
		"stats": b.c.Output.Stats(),
	}.Infof(b.ctx, "Message output has closed")
	b.shutdown(nil)
	return b.getRunErr()
}

// Streams returns a sorted list of stream names that have been registered to
// the Butler.
func (b *Butler) Streams() []types.StreamName {
	return b.streams.Seen()
}

// DrainNamespace prevents any new streams from being registered in the given
// namespace, and waits for all existing streams in that namespace to drain.
//
// If this exits due to context cancelation, it returns the list of stream names
// in the namespace which are still open.
func (b *Butler) DrainNamespace(ctx context.Context, namespace types.StreamName) []types.StreamName {
	ch := b.streams.DrainNamespace(namespace)

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return b.streams.Draining(namespace)
	}
}

// StreamServer is butler's interface for
// go.chromium.org/luci/logdog/client/butler/streamserver
//
// TODO(iannucci): internalize streamserver to butler; there's no need for it to
// be managed by the user.
type StreamServer interface {
	Next() (io.ReadCloser, *logpb.LogStreamDescriptor)
	Close()
}

// AddStreamServer adds a StreamServer to the Butler. This is goroutine-safe
// and may be called anytime before or during Butler execution.
//
// After this call completes, the Butler assumes ownership of the StreamServer.
func (b *Butler) AddStreamServer(streamServer StreamServer) {
	ctx := log.SetField(b.ctx, "streamServer", streamServer)

	log.Debugf(ctx, "Adding stream server.")

	// Pull streams from the streamserver and add them to the Butler.
	streamServerFinishedC := make(chan struct{})
	go func() {
		defer close(streamServerFinishedC)

		defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
			log.Fields{
				"panic.error": p.Reason,
			}.Errorf(b.ctx, "Panic while running StreamServer:\n%s", p.Stack)
			b.shutdown(fmt.Errorf("butler: panic while running StreamServer: %v", p.Reason))
		})

		for {
			rc, config := streamServer.Next()
			if rc == nil {
				log.Debugf(ctx, "StreamServer returned nil stream; terminating.")
				return
			}

			// Add this Stream to the Butler.
			//
			// We run this in a function so we can ensure cleanup on failure.
			if err := b.AddStream(rc, config); err != nil {
				logging.Errorf(ctx, "Failed to add stream %q: %s", config.Name, err)

				if err := rc.Close(); err != nil {
					logging.Warningf(ctx, "Failed to close stream %q: %s", config.Name, err)
				}
			}
		}
	}()

	// Monitor the StreamServer's close signal channel; terminate our server when
	// it's set.
	b.streamServerWG.Add(1)
	go func() {
		defer b.streamServerWG.Done()

		<-b.streamServerStopC
		log.Debugf(ctx, "Stop signal received for StreamServer.")
		streamServer.Close()
		<-streamServerFinishedC
	}()
}

// AddStream adds a Stream to the Butler. This is goroutine-safe.
//
// If no error is returned, the Butler assumes ownership of the supplied stream.
// The stream will be closed when processing is finished.
//
// If an error is occurred, the caller is still the owner of the stream and
// is responsible for closing it.
func (b *Butler) AddStream(rc io.ReadCloser, d *logpb.LogStreamDescriptor) error {
	d = proto.Clone(d).(*logpb.LogStreamDescriptor)
	if d.Timestamp == nil || d.Timestamp.AsTime().IsZero() {
		d.Timestamp = timestamppb.New(clock.Now(b.ctx))
	}
	if err := d.Validate(false); err != nil {
		return err
	}

	// Build per-stream tag map.
	if l := len(b.c.GlobalTags); l > 0 {
		if d.Tags == nil {
			d.Tags = make(map[string]string, l)
		}

		for k, v := range b.c.GlobalTags {
			// Add only global flags that aren't already present (overridden) in
			// stream tags.
			if _, ok := d.Tags[k]; !ok {
				d.Tags[k] = v
			}
		}
	}

	b.maybeAddStreamCallback(d)
	if err := b.streams.RegisterStream(types.StreamName(d.Name)); err != nil {
		logging.WithError(err).Errorf(b.ctx, "failed to register stream")
		return err
	}

	// Build our stream struct.
	streamCtx := log.SetField(b.ctx, "stream", d.Name)
	logging.Infof(streamCtx, "adding stream")
	s := stream{
		log:  logging.Get(streamCtx),
		now:  clock.Get(streamCtx).Now,
		r:    rc,
		c:    rc,
		name: types.StreamName(d.Name),
	}

	// Register this stream with our Bundler. It will take ownership of "d", so
	// we should not use it after this point.
	var err error
	if s.bs, err = b.bundler.Register(d); err != nil {
		return err
	}
	d = nil

	b.streamC <- &s
	return nil
}

func (b *Butler) runStreams(activateC chan struct{}) {
	streamFinishedC := make(chan *stream)
	streamC := b.streamC

	for {
		select {
		case s, ok := <-streamC:
			if !ok {
				// Our streamC has been closed. At this point, we wait for current
				// streams to finish.
				streamC = nil
				continue
			}

			closeOnce := sync.Once{}
			closeStream := func() {
				closeOnce.Do(s.closeStream)
			}

			go func() {
				defer func() {
					// Report that our stream has finished.
					streamFinishedC <- s
				}()
				defer closeStream()

				// Read the stream continuously until we're finished or interrupted.
				finishedC := make(chan struct{})
				go func(s *stream) {
					defer close(finishedC)

					didSomething := false
					for s.readChunk() {
						didSomething = true
					}
					if !didSomething {
						b.finalCallback(string(s.name))
					}
				}(s)

				// Stop processing when either the stream is finished or we are instructed
				// to close the stream via 'streamStopC'.
				select {
				case <-finishedC:
					// The stream has finished on its own.
					break

				case <-b.streamStopC:
					s.log.Debugf("Received stop signal.")
					closeStream()
				}

				// Wait for our stream to finish.
				<-finishedC
			}()

		case s := <-streamFinishedC:
			// A stream has reported that it finished.
			//
			// If this is the last active stream and we've been activated, exit the
			// monitor.
			b.streams.CompleteStream(s.name)
			if b.streams.RunningCount() == 0 && activateC == nil {
				return
			}

		case <-activateC:
			// If we're not managing any streams, then we're done.
			if b.streams.RunningCount() == 0 {
				return
			}

			// Record that we've been activated. Clearing the channel stops the select
			// from hammering the "activateC" case in the period after activation but
			// before our streams are finished.
			activateC = nil
		}
	}
}

// Activate notifies the Butler that its current stream load is sufficient.
// This enables it to exit Run when it reaches a stream count of zero. Prior
// to activation, the Butler would block in Run regardless of stream count.
func (b *Butler) Activate() {
	b.activateOnce.Do(func() {
		close(b.activateC)
	})
}

// shutdown is a goroutine-safe method instructing the Butler to terminate
// with the supplied error code. It may be called more than once, although
// the first supplied error message will be the one returned by Run.
func (b *Butler) shutdown(err error) {
	log.Fields{
		log.ErrorKey: err,
	}.Debugf(b.ctx, "Received 'shutdown()' command; shutting down streams.")

	func() {
		b.shutdownMu.Lock()
		defer b.shutdownMu.Unlock()

		if b.isShutdown {
			// Already shut down.
			return
		}

		// Signal our streams to shutdown prematurely.
		close(b.streamStopC)

		b.runErr = err
		b.isShutdown = true
	}()

	// Activate the Butler, if it hasn't already been activated. The Butler will
	// block pending stream draining, but we've instructed our streams to
	// shutdown prematurely, so this should be reasonably quick.
	b.Activate()
}

// Returns the configured Butler error.
func (b *Butler) getRunErr() error {
	b.shutdownMu.Lock()
	defer b.shutdownMu.Unlock()
	return b.runErr
}
