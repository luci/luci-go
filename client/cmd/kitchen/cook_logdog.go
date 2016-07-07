// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/luci/luci-go/client/internal/logdog/butler"
	"github.com/luci/luci-go/client/internal/logdog/butler/bootstrap"
	"github.com/luci/luci-go/client/internal/logdog/butler/output"
	fileOut "github.com/luci/luci-go/client/internal/logdog/butler/output/file"
	out "github.com/luci/luci-go/client/internal/logdog/butler/output/logdog"
	"github.com/luci/luci-go/client/logdog/annotee"
	"github.com/luci/luci-go/client/logdog/annotee/annotation"
	"github.com/luci/luci-go/client/logdog/butlerlib/streamclient"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/ctxcmd"
	"github.com/luci/luci-go/common/environ"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// runWithLogdogButler rus the supplied command through the a LogDog Butler
// engine instance. This involves:
//	- Determine a LogDog Prefix.
//	- Configuring / setting up the Butler.
//	- Initiating a LogDog Pub/Sub Output, registering with remote server.
//	- Running the recipe process.
//	  - Optionally, hook its output streams up through an Annotee processor.
//	  - Otherwise, wait for the process to finish.
//	- Shut down the Butler instance.
func (c *cookRun) runWithLogdogButler(ctx context.Context, cmd *exec.Cmd) (rc int, err error) {
	_ = auth.Authenticator{}

	authenticator := auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
		Scopes: out.Scopes(),
	})

	// Register and instantiate our LogDog Output.
	var o output.Output
	if c.logdog.filePath == "" {
		ocfg := out.Config{
			Auth:    authenticator,
			Host:    c.logdog.host,
			Project: config.ProjectName(c.logdog.project),
			Prefix:  c.logdog.prefix,
			SourceInfo: []string{
				"Kitchen",
			},
			PublishContext: withNonCancel(ctx),
		}

		var err error
		if o, err = ocfg.Register(ctx); err != nil {
			return 0, errors.Annotate(err).Reason("failed to create LogDog Output instance").Err()
		}
	} else {
		// Debug: Use a file output.
		ocfg := fileOut.Options{
			Path: c.logdog.filePath,
		}
		o = ocfg.New(ctx)
	}
	defer o.Close()

	ncCtx := withNonCancel(ctx)
	b, err := butler.New(ncCtx, butler.Config{
		Output:     o,
		Project:    config.ProjectName(c.logdog.project),
		Prefix:     c.logdog.prefix,
		BufferLogs: true,
	})
	if err != nil {
		err = errors.Annotate(err).Reason("failed to create Butler instance").Err()
		return
	}
	defer func() {
		b.Activate()
		if ierr := b.Wait(); ierr != nil {
			ierr = errors.Annotate(ierr).Reason("failed to Wait() for Butler").Err()
			logAnnotatedErr(ctx, ierr)

			// Promote to function output error if we don't have one yet.
			if err == nil {
				err = ierr
			}
		}
	}()

	// Wrap our incoming command in a CtxCmd.
	proc := ctxcmd.CtxCmd{
		Cmd: cmd,
	}

	var env environ.Env
	if proc.Env != nil {
		env = environ.New(proc.Env)
	} else {
		env = environ.System()
	}

	// Augment our environment with Butler parameters.
	bsEnv := bootstrap.Environment{
		Project: config.ProjectName(c.logdog.project),
		Prefix:  c.logdog.prefix,
	}
	bsEnv.Augment(env)
	proc.Env = env.Sorted()

	// Build pipes for our STDOUT and STDERR streams.
	stdout, err := proc.StdoutPipe()
	if err != nil {
		err = errors.Annotate(err).Reason("failed to get STDOUT pipe").Err()
		return
	}
	defer stdout.Close()

	stderr, err := proc.StderrPipe()
	if err != nil {
		err = errors.Annotate(err).Reason("failed to get STDERR pipe").Err()
		return
	}
	defer stderr.Close()

	// Start our bootstrapped subprocess.
	//
	// We need to consume all of its streams prior to waiting for completion (see
	// exec.Cmd).
	//
	// We'll set up our own cancellation function to help ensure that the process
	// is properly terminated regardless of any encountered errors.
	ctx, cancelFunc := context.WithCancel(ctx)
	if err = proc.Start(ctx); err != nil {
		err = errors.Annotate(err).Reason("failed to start command").Err()
		return
	}
	defer func() {
		// If we've encountered an error, cancel our process.
		if err != nil {
			cancelFunc()
		}

		// Run our command and collect its return code.
		ierr := proc.Wait()
		if waitRC, has := ctxcmd.ExitCode(ierr); has {
			rc = waitRC
		} else {
			ierr = errors.Annotate(ierr).Reason("failed to Wait() for process").Err()
			logAnnotatedErr(ctx, ierr)

			// Promote to function output error if we don't have one yet.
			if err == nil {
				err = ierr
			}
		}
	}()

	if c.logdog.annotee {
		annoteeProcessor := annotee.New(ncCtx, annotee.Options{
			Base:                   "recipes",
			Client:                 streamclient.NewLocal(b),
			Execution:              annotation.ProbeExecution(proc.Args, proc.Env, proc.Dir),
			MetadataUpdateInterval: 30 * time.Second,
			Offline:                false,
			CloseSteps:             true,
		})
		defer func() {
			as := annoteeProcessor.Finish()
			log.Infof(ctx, "Annotations finished:\n%s", proto.MarshalTextString(as.RootStep().Proto()))
		}()

		// Run STDOUT/STDERR streams through the processor. This will block until
		// both streams are closed.
		streams := []*annotee.Stream{
			{
				Reader:           stdout,
				Name:             annotee.STDOUT,
				Annotate:         true,
				StripAnnotations: true,
			},
			{
				Reader:           stderr,
				Name:             annotee.STDERR,
				Annotate:         true,
				StripAnnotations: true,
			},
		}

		// Run the process' output streams through Annotee. This will block until
		// they are all consumed.
		if err = annoteeProcessor.RunStreams(streams); err != nil {
			err = errors.Annotate(err).Reason("failed to process streams through Annotee").Err()
			return
		}
	} else {
		// Get our STDOUT / STDERR stream flags. Tailor them to match Annotee.
		stdoutFlags := annotee.TextStreamFlags(ctx, annotee.STDOUT)
		stderrFlags := annotee.TextStreamFlags(ctx, annotee.STDERR)

		// Wait for our STDOUT / STDERR streams to complete.
		var wg sync.WaitGroup
		stdout = &callbackReadCloser{stdout, wg.Done}
		stderr = &callbackReadCloser{stderr, wg.Done}
		wg.Add(2)

		// Explicitly add these streams to the Butler.
		if err = b.AddStream(stdout, *stdoutFlags.Properties()); err != nil {
			err = errors.Annotate(err).Reason("failed to add STDOUT stream to Butler").Err()
			return
		}
		if err = b.AddStream(stderr, *stderrFlags.Properties()); err != nil {
			err = errors.Annotate(err).Reason("failed to add STDERR stream to Butler").Err()
			return
		}

		// Wait for the streams to be consumed.
		wg.Wait()
	}

	// Our process and Butler instance will be consumed in our teardown
	// defer() statements.
	return
}

// nonCancelContext is a context.Context which deliberately ignores cancellation
// installed in its parent Contexts. This is used to shield the LogDog output
// from having its operations cancelled if the supplied Context is cancelled,
// allowing it to flush.
type nonCancelContext struct {
	base  context.Context
	doneC chan struct{}
}

func withNonCancel(ctx context.Context) context.Context {
	return &nonCancelContext{
		base:  ctx,
		doneC: make(chan struct{}),
	}
}

func (c *nonCancelContext) Deadline() (time.Time, bool)       { return time.Time{}, false }
func (c *nonCancelContext) Done() <-chan struct{}             { return c.doneC }
func (c *nonCancelContext) Err() error                        { return nil }
func (c *nonCancelContext) Value(key interface{}) interface{} { return c.base.Value(key) }

// callbackReadCloser invokes a callback method when closed.
type callbackReadCloser struct {
	io.ReadCloser
	callback func()
}

func (c *callbackReadCloser) Close() error {
	defer c.callback()
	return c.ReadCloser.Close()
}
