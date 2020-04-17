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

package main

import (
	"context"
	"fmt"
	"os"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/resultdb/cmd/result_sink_wrapper/wrapper"
)

const (
	wrapperErrorCode = 1001
)

func main() {
	ctx := context.Background()
	w, err := wrapper.NewWrapper()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(wrapperErrorCode)
	}

	exitCode, err := w.Main(ctx)
	if err != nil {
		logging.Errorf(ctx, "FATAL: %s", err)
		exitCode = wrapperErrorCode
	}
	w.Close()
	os.Exit(exitCode)
}
