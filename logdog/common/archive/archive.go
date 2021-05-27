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

// Package archive constructs a LogDog archive out of log stream components.
// Records are read from the stream and emitted as an archive.
package archive

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"reflect"

	cl "cloud.google.com/go/logging"
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/renderer"
)

// CloudLogging entry has a limit of 256KB in the internal byte representation.
// If an entry is larger, CloudLogging will reject the entry with an error.
//
// To minimize the chance of a LogEntry to exceed the limit, Archive applies the following
// limits to the entry before exporting logpb.LogEntry to CloudLogging.
const (
	// MaxPayload is the maximum size for the payload of a CloudLogging entry.
	//
	// If a single line exceeds the limit in size, the line likely contains a dump of a serialized
	// object, which wouldn't be useful in searches, and the line will get truncated when being
	// exported to CloudLogging.
	MaxPayload = 128 * 1024

	// MaxTagSum is the maximum size sum of tag keys and values that can be attached to
	// a CloudLogging Entry. If the sum exceeds the limit, the stream won't be exported
	// to CloudLogging.
	MaxTagSum = 96 * 1024
)

// CLLogger is a general interface for CloudLogging logger and intended to enable
// unit tests and stub out CloudLogging.
type CLLogger interface {
	Log(cl.Entry)
}

// Manifest is a set of archival parameters.
type Manifest struct {
	// LUCIProject is the LUCI project for the stream.
	LUCIProject string

	// Desc is the logpb.LogStreamDescriptor for the stream.
	Desc *logpb.LogStreamDescriptor
	// Source is the LogEntry Source for the stream.
	Source renderer.Source

	// LogWriter, if not nil, is the Writer to which the log stream record stream
	// will be written.
	LogWriter io.Writer
	// IndexWriter, if not nil, is the Writer to which the log stream Index
	// protobuf stream will be written.
	IndexWriter io.Writer

	// StreamIndexRange, if >0, is the maximum number of log entry stream indices
	// in between successive index entries.
	//
	// If no index constraints are set, an index entry will be emitted for each
	// LogEntry.
	StreamIndexRange int
	// PrefixIndexRange, if >0, is the maximum number of log entry prefix indices
	// in between successive index entries.
	PrefixIndexRange int
	// ByteRange, if >0, is the maximum number of log entry bytes in between
	// successive index entries.
	ByteRange int

	// Logger, if not nil, will be used to log status during archival.
	Logger logging.Logger

	// CloudLogger, if not nil, will be used to export archived log entries to Cloud Logging.
	CloudLogger CLLogger

	// sizeFunc is a size method override used for testing.
	sizeFunc func(proto.Message) int
}

func (m *Manifest) logger() logging.Logger {
	if m.Logger == nil ||
		(reflect.ValueOf(m.Logger).Kind() == reflect.Ptr &&
			reflect.ValueOf(m.Logger).IsNil()) {
		return logging.Null
	}
	return m.Logger
}

// Archive performs the log archival described in the supplied Manifest.
func Archive(m Manifest) error {
	// Wrap our log source in a safeLogEntrySource to protect our index order.
	m.Source = &safeLogEntrySource{
		Manifest: &m,
		Source:   m.Source,
	}

	// If no constraints are applied, index every LogEntry.
	if m.StreamIndexRange <= 0 && m.PrefixIndexRange <= 0 && m.ByteRange <= 0 {
		m.StreamIndexRange = 1
	}

	if m.LogWriter == nil {
		return nil
	}

	// If we're constructing an index, allocate a stateful index builder.
	var idx *indexBuilder
	if m.IndexWriter != nil {
		idx = &indexBuilder{
			Manifest: &m,
			index: logpb.LogIndex{
				Desc: m.Desc,
			},
			sizeFunc: m.sizeFunc,
		}
	}

	// Compute a hash to be used as the ID of the stream in Cloud Logging.
	sha := sha256.New()
	sha.Write([]byte(m.LUCIProject))
	sha.Write([]byte(m.Desc.Prefix))
	sha.Write([]byte(m.Desc.Name))
	streamIDHash := sha.Sum(nil)

	return parallel.FanOutIn(func(taskC chan<- func() error) {
		logC := make(chan *logpb.LogEntry)

		taskC <- func() error {
			if err := archiveLogs(m.LogWriter, m.Desc, logC, idx, m.CloudLogger, streamIDHash, m.logger()); err != nil {
				return err
			}

			// If we're building an index, emit it now that the log stream has
			// finished.
			if idx != nil {
				return idx.emit(m.IndexWriter)
			}
			return nil
		}

		// Iterate through all of our Source's logs and process them.
		taskC <- func() error {
			defer close(logC)

			for {
				le, err := m.Source.NextLogEntry()
				if le != nil {
					logC <- le
				}

				switch err {
				case nil:
				case io.EOF:
					return nil
				default:
					return err
				}
			}
		}
	})
}

func archiveLogs(w io.Writer, d *logpb.LogStreamDescriptor, logC <-chan *logpb.LogEntry, idx *indexBuilder, cloudLogger CLLogger, streamIDHash []byte, logger logging.Logger) error {
	offset := int64(0)
	out := func(pb proto.Message) error {
		d, err := proto.Marshal(pb)
		if err != nil {
			return err
		}

		count, err := recordio.WriteFrame(w, d)
		offset += int64(count)
		return err
	}

	isCLDisabled := (cloudLogger == nil ||
		(reflect.ValueOf(cloudLogger).Kind() == reflect.Ptr &&
			reflect.ValueOf(cloudLogger).IsNil()))

	if !isCLDisabled {
		tsum := 0
		for k, v := range d.GetTags() {
			tsum += len(k)
			tsum += len(v)

			if tsum > MaxTagSum {
				logger.Errorf("sum(tags) > %d; skipping the stream for CloudLogging export", MaxTagSum)
				isCLDisabled = true
				break
			}
		}
	}
	// Start with our descriptor protobuf. Defer error handling until later, as
	// we are still responsible for draining "logC".
	err := out(d)

	var buf bytes.Buffer
	var le *logpb.LogEntry
	// seq is the number of cl.Entry(s) exported and used to assign a unique insertID
	// to each of them, in case multiple cl.Entry(s) were produced from a single logpb.LogEntry.
	seq := 0

	for le = range logC {
		if err == nil {
			// Add this LogEntry to our index, noting the current offset.
			if idx != nil {
				idx.addLogEntry(le, offset)
			}
			err = out(le)
		}

		// Skip CloudLogging export, if inteded.
		lines := le.GetText().GetLines()
		if isCLDisabled || len(lines) == 0 {
			continue
		}

		if lines[0].Delimiter == "" {
			if writable := MaxPayload - buf.Len(); writable > 0 {
				buf.Write(lines[0].Value[:min(writable, len(lines[0].Value))])
			}
			continue
		}

		// Drop the first complete line if it is going to be too long w/ the carryover.
		if writable := MaxPayload - buf.Len(); writable < lineLength(lines[0]) {
			if writable > 0 {
				buf.Write(lines[0].Value[:writable])
			}
			cloudLogger.Log(toCloudLogEntry(d, le, streamIDHash, seq, buf.String()))
			seq += 1
			buf.Reset()

			lines = lines[1:]
			if len(lines) == 0 {
				continue
			}
		}

		// handle the remaining complete lines.
		var pl *logpb.Text_Line
		if ll := lines[len(lines)-1]; ll.Delimiter == "" {
			pl = ll
			lines = lines[:len(lines)-1]
		}
		for i, line := range lines {
			// flush the preceeding lines, if the payload is going to be too long.
			if buf.Len() > 0 && buf.Len()+lineLength(line) > MaxPayload {
				cloudLogger.Log(toCloudLogEntry(d, le, streamIDHash, seq, buf.String()))
				buf.Reset()
				seq += 1
			}

			// the line itself is too big?
			if lineLength(line) > MaxPayload {
				payload := line.Value[:min(MaxPayload, len(line.Value))]
				cloudLogger.Log(toCloudLogEntry(d, le, streamIDHash, seq, string(payload)))
				seq += 1
			} else {
				buf.Write(line.Value)
				if i < len(lines)-1 {
					buf.WriteString(line.Delimiter)
				}
			}
		}

		// flush the leftover.
		//
		// At this stage, the leftover are always complete lines, and complete lines are not
		// carried over to the next LogEntry.
		if buf.Len() > 0 {
			cloudLogger.Log(toCloudLogEntry(d, le, streamIDHash, seq, buf.String()))
			buf.Reset()
			seq += 1
		}

		// keep the partial line to carry it over to the next LogEntry.
		if pl != nil {
			writable := MaxPayload - buf.Len()
			if n := min(writable, len(pl.Value)); n > 0 {
				buf.Write(pl.Value[:n])
			}
		}
	}

	// Any leftover with an incomplete line?
	if buf.Len() > 0 {
		logger.Infof("A partial line at the end of a stream (%d bytes)", buf.Len())
		cloudLogger.Log(toCloudLogEntry(d, le, streamIDHash, seq, buf.String()))
	}
	return err
}

func toCloudLogEntry(d *logpb.LogStreamDescriptor, le *logpb.LogEntry, streamIDHash []byte, seq int, payload string) cl.Entry {
	ts := d.Timestamp.AsTime()
	if le.TimeOffset != nil {
		ts = ts.Add(le.TimeOffset.AsDuration())
	}

	return cl.Entry{
		Payload:   payload,
		Timestamp: ts,

		// InsertID uniquely identifies each LogEntry to dedup entries in CloudLogging.
		InsertID: fmt.Sprintf("%x/%d", streamIDHash, seq),

		// Set the Trace field with the streamID hash so that Log entries can be grouped together
		// by the streamID.
		Trace: fmt.Sprintf("%x", streamIDHash),
	}
}

func min(lhs, rhs int) int {
	if lhs < rhs {
		return lhs
	}
	return rhs
}

func lineLength(l *logpb.Text_Line) int {
	return len(l.Value) + len(l.Delimiter)
}
