// Copyright 2019 The LUCI Authors.
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

package directory

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/jsonpb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/logdog/api/logpb"
)

// stream is the stateful output for a single log stream.
type stream struct {
	curFile *os.File // nil if no file open

	basePath      string
	fname         string
	isDatagram    bool
	datagramCount int
}

func newStream(basePath string, desc *logpb.LogStreamDescriptor) (*stream, error) {
	relPath := filepath.Clean(desc.Name)
	dir, fname := filepath.Split(relPath)
	basePath = filepath.Join(basePath, dir)

	_ = os.MkdirAll(basePath, 0750)
	metaF, err := os.Create(filepath.Join(basePath, ".meta."+fname))
	if err != nil {
		return nil, errors.Fmt("opening meta file for %s: %w", relPath, err)
	}
	defer metaF.Close()
	err = (&jsonpb.Marshaler{
		Indent:   "  ",
		OrigName: true,
	}).Marshal(metaF, desc)
	if err != nil {
		return nil, errors.Fmt("writing meta file for %s: %w", relPath, err)
	}

	ret := stream{basePath: basePath, fname: fname}
	if desc.StreamType == logpb.StreamType_DATAGRAM {
		ret.isDatagram = true
	} else {
		ret.curFile, err = os.Create(filepath.Join(basePath, fname))
	}
	return &ret, err
}

func (s *stream) getCurFile() (*os.File, error) {
	if s.curFile != nil {
		return s.curFile, nil
	}

	if !s.isDatagram {
		return nil, errors.New(
			"cannot call getCurFile for a non-datagram with a closed file")
	}

	var err error
	s.curFile, err = os.Create(
		filepath.Join(s.basePath, fmt.Sprintf("_%05d.%s", s.datagramCount, s.fname)))
	if err != nil {
		return nil, errors.Fmt("could not open %d'th datagram of %s: %w",
			s.datagramCount, filepath.Join(s.basePath, s.fname), err)
	}
	s.datagramCount++

	return s.curFile, nil
}

func (s *stream) closeCurFile() {
	if s.curFile != nil {
		s.curFile.Close()
		s.curFile = nil
	}
}

// ingestBundleEntry writes the data from `be` to disk
//
// Returns closed == true if `be` was terminal and the stream can be closed now.
func (s *stream) ingestBundleEntry(be *logpb.ButlerLogBundle_Entry) (closed bool, err error) {
	for _, le := range be.GetLogs() {
		curFile, err := s.getCurFile()
		if err != nil {
			return false, err
		}

		switch x := le.Content.(type) {
		case *logpb.LogEntry_Datagram:
			dg := x.Datagram
			_, err = s.curFile.Write(dg.Data)
			if err == nil {
				if dg.Partial == nil || dg.Partial.Last {
					s.closeCurFile()
				}
			}
		case *logpb.LogEntry_Text:
			for _, line := range x.Text.Lines {
				_, err = curFile.Write(line.Value)
				if err == nil {
					_, err = curFile.WriteString("\n")
				}
			}
		case *logpb.LogEntry_Binary:
			_, err = curFile.Write(x.Binary.Data)
		}

		if err != nil {
			return false, err
		}
	}
	if be.Terminal {
		s.Close()
		return true, nil
	}
	return false, nil
}

func (s *stream) Close() {
	s.closeCurFile()
}
