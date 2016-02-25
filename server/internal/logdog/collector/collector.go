// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package collector

import (
	"bytes"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logdog/butlerproto"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/server/internal/logdog/collector/coordinator"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

// Collector is a stateful object responsible for ingesting LogDog logs,
// registering them with a Coordinator, and stowing them in short-term storage
// for streaming and processing.
//
// A Collector's Close should be called when finished to release any internal
// resources.
type Collector struct {
	// Coordinator is used to interface with the Coordinator client.
	//
	// On production systems, this should wrapped with a caching client (see
	// the stateCache sub-package) to avoid overwhelming the server.
	Coordinator coordinator.Coordinator

	// Storage is the backing store to use.
	Storage storage.Storage

	// StreamStateCacheExpire is the maximum amount of time that a cached stream
	// state entry is valid. If zero, DefaultStreamStateCacheExpire will be used.
	StreamStateCacheExpire time.Duration

	// MaxParallelBundles is the maximum number of log entry bundles per message
	// to handle in parallel. If <= 0, no maximum will be applied.
	MaxParallelBundles int
	// MaxIngestWorkers is the maximum number of ingest worker goroutines that
	// will operate at a time. If <= 0, no maximum will be applied.
	MaxIngestWorkers int

	// initOnce is used to ensure that the Collector's internal state is
	// initialized at most once.
	initOnce sync.Once
	// runner is the Runner that will be used for ingest. It will be configured
	// based on the supplied MaxIngestWorkers parameter.
	//
	// Internally, runner must not be used by tasks that themselves use the
	// runner, else deadlock could occur.
	runner *parallel.Runner
}

// init initializes the operational state of the Collector. It must be called
// internally at the beginning of any exported method that uses that state.
func (c *Collector) init() {
	c.initOnce.Do(func() {
		c.runner = &parallel.Runner{
			Sustained: c.MaxIngestWorkers,
			Maximum:   c.MaxIngestWorkers,
		}
	})
}

// Process ingests an encoded ButlerLogBundle message, registering it with
// the LogDog Coordinator and stowing it in a temporary Storage for streaming
// retrieval.
//
// If a transient error occurs during ingest, Process will return an error.
// If no error occurred, or if there was an error with the input data, no error
// will be returned.
func (c *Collector) Process(ctx context.Context, msg []byte) error {
	c.init()

	pr := butlerproto.Reader{}
	if err := pr.Read(bytes.NewReader(msg)); err != nil {
		log.Errorf(log.SetError(ctx, err), "Failed to unpack message.")
		return nil
	}
	if pr.Metadata.ProtoVersion != logpb.Version {
		log.Fields{
			"messageProtoVersion": pr.Metadata.ProtoVersion,
			"currentProtoVersion": logpb.Version,
		}.Errorf(ctx, "Unknown protobuf version.")
		return nil
	}
	if pr.Bundle == nil {
		log.Errorf(ctx, "Protocol message did not contain a Butler bundle.")
		return nil
	}

	// If we're logging INFO or higher, log the ranges that this bundle
	// represents.
	if log.IsLogging(ctx, log.Info) {
		for i, entry := range pr.Bundle.Entries {
			fields := log.Fields{
				"index": i,
				"path":  entry.GetDesc().Path(),
			}
			if entry.Terminal {
				fields["terminalIndex"] = entry.TerminalIndex
			}
			if logs := entry.GetLogs(); len(logs) > 0 {
				fields["logStart"] = logs[0].StreamIndex
				fields["logEnd"] = logs[len(logs)-1].StreamIndex
			}

			fields.Infof(ctx, "Processing log bundle entry.")
		}
	}

	// If there are no entries, there is nothing to do.
	if len(pr.Bundle.Entries) == 0 {
		return nil
	}

	// Define our logWork template. This will be cloned for each ingested log
	// stream.
	lw := logWork{
		md: pr.Metadata,
		b:  pr.Bundle,
	}

	// Handle each bundle entry in parallel. We will use a separate work pool
	// here so that top-level bundle dispatch can't deadlock the processing tasks.
	err := parallel.WorkPool(c.MaxParallelBundles, func(taskC chan<- func() error) {
		for _, be := range pr.Bundle.Entries {
			lw := lw
			lw.be = be
			taskC <- func() error {
				return c.processLogStream(ctx, &lw)
			}
		}
	})
	if err != nil {
		if hasTransientError(err) && !errors.IsTransient(err) {
			// err has a nested transient error; propagate that to top.
			err = errors.WrapTransient(err)
		}
		return err
	}
	return nil
}

// Close releases any internal resources and blocks pending the completion of
// any outstanding operations. After Close, no new Process calls may be made.
func (c *Collector) Close() {
	c.init()

	c.runner.Close()
}

// logWork is a cumulative set of read-only state passed around by value for log
// processing.
type logWork struct {
	// md is the metadata associated with the overall message.
	md *logpb.ButlerMetadata
	// b is the Butler bundle.
	b *logpb.ButlerLogBundle
	// be is the Bundle entry.
	be *logpb.ButlerLogBundle_Entry
	// path is the constructed path of the stream being processed.
	path types.StreamPath
	// le is the LogEntry in the bundle entry.
	le *logpb.LogEntry
}

// processLogStream processes an individual set of log messages belonging to the
// same log stream.
func (c *Collector) processLogStream(ctx context.Context, lw *logWork) error {
	if err := lw.be.Desc.Validate(true); err != nil {
		log.Errorf(log.SetError(ctx, err), "Invalid log stream descriptor.")
		return nil
	}
	lw.path = types.StreamName(lw.be.Desc.Prefix).Join(types.StreamName(lw.be.Desc.Name))
	ctx = log.SetField(ctx, "path", lw.path)

	if len(lw.be.Secret) == 0 {
		log.Errorf(ctx, "Missing secret.")
		return nil
	}

	// Fetch our cached/remote state. This will replace our state object with the
	// fetched state, so any future calls will need to re-set the Secret value.
	// TODO: Use timeout?
	state, err := c.Coordinator.RegisterStream(ctx, &coordinator.LogStreamState{
		Path:         lw.path,
		Secret:       types.StreamSecret(lw.be.Secret),
		ProtoVersion: lw.md.ProtoVersion,
	}, lw.be.Desc)
	if err != nil {
		log.WithError(err).Errorf(ctx, "Failed to get/register current stream state.")
		return err
	}

	// Does the log stream's secret match the expected secret?
	if !bytes.Equal(lw.be.Secret, []byte(state.Secret)) {
		log.Errorf(log.SetFields(ctx, log.Fields{
			"secret":         lw.be.Secret,
			"expectedSecret": state.Secret,
		}), "Log entry has incorrect secret.")
		return nil
	}

	if state.Archived {
		log.Infof(ctx, "Skipping message bundle for archived stream.")
		return nil
	}
	if state.Purged {
		log.Infof(ctx, "Skipping message bundle for purged stream.")
		return nil
	}

	// Update our terminal index if we have one.
	//
	// Note that even if our cached value is marked terminal, we could have failed
	// to push the terminal index to the Coordinator, so we will not refrain from
	// pushing every terminal index encountered regardless of cache state.
	if lw.be.Terminal {
		tidx := types.MessageIndex(lw.be.TerminalIndex)
		log.Fields{
			"value": tidx,
		}.Debugf(ctx, "Bundle includes a terminal index.")

		if state.TerminalIndex < 0 {
			state.TerminalIndex = tidx
		} else if state.TerminalIndex != tidx {
			log.Fields{
				"cachedIndex": state.TerminalIndex,
				"bundleIndex": tidx,
			}.Warningf(ctx, "Cached terminal index disagrees with state.")
		}
	}

	// In parallel, load the log entries into Storage. Throttle this with our
	// ingest semaphore.
	return errors.MultiErrorFromErrors(c.runner.Run(func(taskC chan<- func() error) {
		for i, le := range lw.be.Logs {
			i, le := i, le

			// Store this LogEntry
			taskC <- func() error {
				if err := le.Validate(lw.be.Desc); err != nil {
					log.Fields{
						log.ErrorKey: err,
						"index":      i,
					}.Warningf(ctx, "Discarding invalid log entry.")
					return nil
				}

				if state.TerminalIndex >= 0 && types.MessageIndex(le.StreamIndex) > state.TerminalIndex {
					log.Fields{
						"index":         le.StreamIndex,
						"terminalIndex": state.TerminalIndex,
					}.Warningf(ctx, "Stream is terminated before log entry; discarding.")
					return nil
				}

				lw := *lw
				lw.le = le
				return c.processLogEntry(ctx, &lw)
			}
		}

		// If our bundle entry is terminal, we have an additional task of reporting
		// this to the Coordinator.
		if lw.be.Terminal {
			taskC <- func() error {
				// Sentinel task: Update the terminal bundle state.
				state := *state
				state.TerminalIndex = types.MessageIndex(lw.be.TerminalIndex)

				log.Fields{
					"terminalIndex": state.TerminalIndex,
				}.Infof(ctx, "Received terminal log; updating Coordinator state.")

				if err := c.Coordinator.TerminateStream(ctx, &state); err != nil {
					log.WithError(err).Errorf(ctx, "Failed to set stream terminal index.")
					return err
				}
				return nil
			}
		}
	}))
}

func (c *Collector) processLogEntry(ctx context.Context, lw *logWork) error {
	data, err := proto.Marshal(lw.le)
	if err != nil {
		log.WithError(err).Errorf(ctx, "Failed to marshal log entry.")
		return err
	}

	// Post the log to storage.
	err = c.Storage.Put(&storage.PutRequest{
		Path:  lw.path,
		Index: types.MessageIndex(lw.le.StreamIndex),
		Value: data,
	})

	// If the log entry already exists, consider the "put" successful.
	//
	// All Storage errors are considered transient, as they are safe and
	// data-agnostic.
	if err != nil && err != storage.ErrExists {
		log.WithError(err).Errorf(ctx, "Failed to load log entry into Storage.")
		return errors.WrapTransient(err)
	}
	return nil
}

// wrapMultiErrorTransient wraps an error in a TransientError wrapper.
//
// If the error is nil, it will return nil. If the error is already transient,
// it will be directly returned. If the error is a MultiError, its sub-errors
// will be evaluated and wrapped in a TransientError if any of its sub-errors
// are transient errors.
func hasTransientError(err error) bool {
	if merr, ok := err.(errors.MultiError); ok {
		for _, e := range merr {
			if hasTransientError(e) {
				return true
			}
		}
		return false
	}

	return errors.IsTransient(err)
}
