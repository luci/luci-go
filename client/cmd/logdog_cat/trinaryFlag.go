// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"

	"github.com/luci/luci-go/common/flag/flagenum"
	"github.com/luci/luci-go/common/logdog/coordinator"
)

type trinaryValue coordinator.QueryTrinary

var _ flag.Value = (*trinaryValue)(nil)

var trinaryFlagEnum = flagenum.Enum{
	"":    coordinator.Both,
	"yes": coordinator.Yes,
	"no":  coordinator.No,
}

func (v trinaryValue) Trinary() coordinator.QueryTrinary {
	return coordinator.QueryTrinary(v)
}

func (v *trinaryValue) String() string {
	return trinaryFlagEnum.FlagString(*v)
}

func (v *trinaryValue) Set(s string) error {
	var tv coordinator.QueryTrinary
	if err := trinaryFlagEnum.FlagSet(&tv, s); err != nil {
		return err
	}
	*v = trinaryValue(tv)
	return nil
}
