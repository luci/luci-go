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

package streamproto

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"go.chromium.org/luci/common/flag/flagenum"
)

// TeeType is an enumeration of tee configuration options.
type TeeType uint

var _ interface {
	flag.Value
	json.Marshaler
	json.Unmarshaler
} = (*TeeType)(nil)

const (
	// TeeNone indicates that no teeing should be performed on the stream.
	TeeNone TeeType = iota
	// TeeStdout configures the stream to tee data through STDOUT.
	TeeStdout
	// TeeStderr configures the stream to tee data through STDERR.
	TeeStderr
)

// Writer returns the io.Writer object
func (t TeeType) Writer() io.Writer {
	switch t {
	case TeeNone:
		return nil

	case TeeStdout:
		return os.Stdout

	case TeeStderr:
		return os.Stderr

	default:
		panic(fmt.Errorf("streamproto: unknown tee type [%v]", t))
	}
}

var (
	// TeeTypeFlagEnum is a flag- and JSON-compatible enumeration mapping
	// TeeType configuration strings to their underlying TeeType values.
	TeeTypeFlagEnum = flagenum.Enum{
		"none":   TeeNone,
		"stdout": TeeStdout,
		"stderr": TeeStderr,
	}
)

// Set implements flag.Value.
func (t *TeeType) Set(v string) error {
	return TeeTypeFlagEnum.FlagSet(t, v)
}

// String implements flag.Value.
func (t *TeeType) String() string {
	return TeeTypeFlagEnum.FlagString(t)
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *TeeType) UnmarshalJSON(data []byte) error {
	return TeeTypeFlagEnum.JSONUnmarshal(t, data)
}

// MarshalJSON implements json.Marshaler.
func (t TeeType) MarshalJSON() ([]byte, error) {
	return TeeTypeFlagEnum.JSONMarshal(t)
}
