// Copyright 2016 The LUCI Authors.
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

package main

import (
	"flag"

	"go.chromium.org/luci/common/flag/flagenum"

	"go.chromium.org/luci/grpc/prpc"
)

type formatFlag prpc.Format

const formatFlagJSONPB = formatFlag(prpc.FormatJSONPB)

// formatFlag is a CLI-settable pRPC format flag.
var formatFlagMap = flagenum.Enum{
	"binary": formatFlag(prpc.FormatBinary),
	"json":   formatFlagJSONPB,
	"text":   formatFlag(prpc.FormatText),
}

var _ flag.Value = (*formatFlag)(nil)

func (f formatFlag) Format() prpc.Format {
	return prpc.Format(f)
}

func (f *formatFlag) String() string     { return formatFlagMap.FlagString(f) }
func (f *formatFlag) Set(v string) error { return formatFlagMap.FlagSet(f, v) }
