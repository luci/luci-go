// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"errors"
	"io"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/config"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/logdog/common/fetcher"
	"github.com/luci/luci-go/logdog/common/renderer"
	"github.com/luci/luci-go/logdog/common/types"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"
)

var errDatagramNotSupported = errors.New("datagram not supported")

type catCommandRun struct {
	subcommands.CommandRunBase

	index      int64
	count      int64
	buffer     int
	fetchSize  int
	fetchBytes int
	raw        bool
}

func newCatCommand() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "cat",
		ShortDesc: "Write log stream to STDOUT.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &catCommandRun{}

			cmd.Flags.Int64Var(&cmd.index, "index", 0, "Starting index.")
			cmd.Flags.Int64Var(&cmd.count, "count", 0, "The number of log entries to fetch.")
			cmd.Flags.IntVar(&cmd.buffer, "buffer", 64,
				"The size of the read buffer. A smaller buffer will more responsive while streaming, whereas "+
					"a larger buffer will have higher throughput.")
			cmd.Flags.IntVar(&cmd.fetchSize, "fetch-size", 0, "Constrains the number of log entries to fetch per request.")
			cmd.Flags.IntVar(&cmd.fetchBytes, "fetch-bytes", 0, "Constrains the number of bytes to fetch per request.")
			cmd.Flags.BoolVar(&cmd.raw, "raw", false,
				"Reproduce original log stream, instead of attempting to render for humans.")
			return cmd
		},
	}
}

func (cmd *catCommandRun) Run(scApp subcommands.Application, args []string) int {
	a := scApp.(*application)

	if len(args) == 0 {
		log.Errorf(a, "At least one log path must be supplied.")
		return 1
	}

	// Validate and construct our cat paths.
	paths := make([]*streamPath, len(args))
	for i, arg := range args {
		// User-friendly: trim any leading or trailing slashes from the path.
		project, path, _, err := a.splitPath(arg)
		if err != nil {
			log.WithError(err).Errorf(a, "Invalid path specifier.")
			return 1
		}

		sp := streamPath{project, types.StreamPath(path)}
		if err := sp.path.Validate(); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"index":      i,
				"project":    sp.project,
				"path":       sp.path,
			}.Errorf(a, "Invalid command-line stream path.")
			return 1
		}

		paths[i] = &sp
	}
	if cmd.buffer <= 0 {
		log.Fields{
			"value": cmd.buffer,
		}.Errorf(a, "Buffer size must be >0.")
	}

	tctx, _ := a.timeoutCtx(a)
	for i, sp := range paths {
		if err := cmd.catPath(tctx, a.coord, sp); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"project":    sp.project,
				"path":       sp.path,
				"index":      i,
			}.Errorf(a, "Failed to fetch log stream.")

			if err == context.DeadlineExceeded {
				return 2
			}
			return 1
		}
	}

	return 0
}

// streamPath is a single path to fetch.
type streamPath struct {
	project config.ProjectName
	path    types.StreamPath
}

func (cmd *catCommandRun) catPath(c context.Context, coord *coordinator.Client, sp *streamPath) error {
	// Pull stream information.
	src := coordinatorSource{
		stream: coord.Stream(sp.project, sp.path),
	}
	src.tidx = -1 // Must be set to probe for state.

	f := fetcher.New(c, fetcher.Options{
		Source:      &src,
		Index:       types.MessageIndex(cmd.index),
		Count:       cmd.count,
		BufferCount: cmd.fetchSize,
		BufferBytes: int64(cmd.fetchBytes),
	})

	rend := renderer.Renderer{
		Source: f,
		Raw:    cmd.raw,
		DatagramWriter: func(w io.Writer, dg []byte) bool {
			desc, err := src.descriptor()
			if err != nil {
				log.WithError(err).Errorf(c, "Failed to get stream descriptor.")
				return false
			}
			return getDatagramWriter(c, desc)(w, dg)
		},
	}
	if _, err := io.CopyBuffer(os.Stdout, &rend, make([]byte, cmd.buffer)); err != nil {
		return err
	}
	return nil
}

// getDatagramWriter returns a datagram writer function that can be used as a
// Renderer's DatagramWriter. The writer is bound to desc.
func getDatagramWriter(c context.Context, desc *logpb.LogStreamDescriptor) renderer.DatagramWriter {

	return func(w io.Writer, dg []byte) bool {
		var pb proto.Message
		switch desc.ContentType {
		case milo.ContentTypeAnnotations:
			mp := milo.Step{}
			if err := proto.Unmarshal(dg, &mp); err != nil {
				log.WithError(err).Errorf(c, "Failed to unmarshal datagram data.")
				return false
			}
			pb = &mp

		default:
			return false
		}

		if err := proto.MarshalText(w, pb); err != nil {
			log.WithError(err).Errorf(c, "Failed to marshal datagram as text.")
			return false
		}

		return true
	}
}
