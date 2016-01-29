// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"

	"github.com/luci/luci-go/common/flag/flagenum"
	"github.com/luci/luci-go/common/prpc"
)

type formatFlag prpc.Format

// formatFlag is a CLI-settable pRPC format flag.
var formatFlagMap = flagenum.Enum{
	"json":   formatFlag(prpc.FormatJSONPB),
	"binary": formatFlag(prpc.FormatBinary),
	"text":   formatFlag(prpc.FormatText),
}

var _ flag.Value = (*formatFlag)(nil)

func (f formatFlag) Format() prpc.Format {
	return prpc.Format(f)
}

func (f *formatFlag) String() string     { return formatFlagMap.FlagString(f) }
func (f *formatFlag) Set(v string) error { return formatFlagMap.FlagSet(f, v) }
