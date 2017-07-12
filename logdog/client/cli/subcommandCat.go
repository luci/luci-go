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

package cli

import (
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/flag/flagenum"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/logdog/common/fetcher"
	"github.com/luci/luci-go/logdog/common/renderer"
	"github.com/luci/luci-go/logdog/common/types"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"
)

var errDatagramNotSupported = errors.New("datagram not supported")

type timestampsFlag string

const (
	timestampsOff   timestampsFlag = ""
	timestampsLocal timestampsFlag = "local"
	timestampsUTC   timestampsFlag = "utc"
)

func (t *timestampsFlag) Set(v string) error { return timestampFlagEnum.FlagSet(t, v) }
func (t *timestampsFlag) String() string     { return timestampFlagEnum.FlagString(t) }

var timestampFlagEnum = flagenum.Enum{
	"":      timestampsOff,
	"local": timestampsLocal,
	"utc":   timestampsUTC,
}

type catCommandRun struct {
	subcommands.CommandRunBase

	index      int64
	count      int64
	buffer     int
	fetchSize  int
	fetchBytes int
	raw        bool

	timestamps      timestampsFlag
	showStreamIndex bool
}

func newCatCommand() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "cat",
		ShortDesc: "Write log stream to STDOUT.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &catCommandRun{}

			cmd.Flags.Int64Var(&cmd.index, "index", 0, "Starting index.")
			cmd.Flags.Int64Var(&cmd.count, "count", 0, "The number of log entries to fetch.")
			cmd.Flags.Var(&cmd.timestamps, "timestamps",
				"When rendering text logs, prefix them with their timestamps. Options are: "+timestampFlagEnum.Choices())
			cmd.Flags.BoolVar(&cmd.showStreamIndex, "show-stream-index", false,
				"When rendering text logs, show their stream index.")
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

func (cmd *catCommandRun) Run(scApp subcommands.Application, args []string, _ subcommands.Env) int {
	a := scApp.(*application)

	if len(args) == 0 {
		log.Errorf(a, "At least one log path must be supplied.")
		return 1
	}

	// Validate and construct our cat addresses.
	addrs := make([]*types.StreamAddr, len(args))
	for i, arg := range args {
		// If the address parses as a URL, use it directly.
		var err error
		if addrs[i], err = types.ParseURL(arg); err == nil {
			continue
		}

		// User-friendly: trim any leading or trailing slashes from the path.
		project, path, _, err := a.splitPath(arg)
		if err != nil {
			log.WithError(err).Errorf(a, "Invalid path specifier.")
			return 1
		}

		addr := types.StreamAddr{Project: project, Path: types.StreamPath(path)}
		if err := addr.Path.Validate(); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"index":      i,
				"project":    addr.Project,
				"path":       addr.Path,
			}.Errorf(a, "Invalid command-line stream path.")
			return 1
		}

		if addr.Host, err = a.resolveHost(""); err != nil {
			err = errors.Annotate(err, "failed to resolve host: %q", addr.Host).Err()
			errors.Log(a, err)
			return 1
		}

		addrs[i] = &addr
	}
	if cmd.buffer <= 0 {
		log.Fields{
			"value": cmd.buffer,
		}.Errorf(a, "Buffer size must be >0.")
	}

	coords := make(map[string]*coordinator.Client, len(addrs))
	for _, addr := range addrs {
		if _, ok := coords[addr.Host]; ok {
			continue
		}

		var err error
		if coords[addr.Host], err = a.coordinatorClient(addr.Host); err != nil {
			err = errors.Annotate(err, "failed to create Coordinator client for %q", addr.Host).Err()

			errors.Log(a, err)
			return 1
		}
	}

	tctx, _ := a.timeoutCtx(a)
	for i, addr := range addrs {
		if err := cmd.catPath(tctx, coords[addr.Host], addr); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"project":    addr.Project,
				"path":       addr.Path,
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

func (cmd *catCommandRun) catPath(c context.Context, coord *coordinator.Client, addr *types.StreamAddr) error {
	// Pull stream information.
	src := coordinatorSource{
		stream: coord.Stream(addr.Project, addr.Path),
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
		TextPrefix: func(le *logpb.LogEntry, line *logpb.Text_Line) string {
			desc, err := src.descriptor()
			if err != nil {
				log.WithError(err).Errorf(c, "Failed to get text prefix descriptor.")
				return ""
			}
			return cmd.getTextPrefix(desc, le)
		},
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

func (cmd *catCommandRun) getTextPrefix(desc *logpb.LogStreamDescriptor, le *logpb.LogEntry) string {
	var parts []string
	if cmd.timestamps != timestampsOff {
		ts := google.TimeFromProto(desc.Timestamp)
		ts = ts.Add(google.DurationFromProto(le.TimeOffset))
		switch cmd.timestamps {
		case timestampsLocal:
			parts = append(parts, ts.Local().Format(time.StampMilli))

		case timestampsUTC:
			parts = append(parts, ts.UTC().Format(time.StampMilli))
		}
	}

	if cmd.showStreamIndex {
		parts = append(parts, strconv.FormatUint(le.StreamIndex, 10))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ") + "| "
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
