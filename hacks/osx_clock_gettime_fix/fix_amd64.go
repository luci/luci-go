// Copyright 2021 The LUCI Authors.
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

//go:build darwin && amd64 && go1.16
// +build darwin,amd64,go1.16

package osx_clock_gettime_fix

import (
	"fmt"
	"os"
)

//go:cgo_import_dynamic libc_gettimeofday gettimeofday "/usr/lib/libSystem.B.dylib"

func init() {
	if os.Getenv("LUCI_GO_CHECK_HACKS") == "1" {
		fmt.Fprintf(os.Stderr, "LUCI_GO_CHECK_HACKS: osx_clock_gettime_fix is enabled\n")
	}
}
