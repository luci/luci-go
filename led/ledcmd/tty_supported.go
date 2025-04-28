// Copyright 2023 The LUCI Authors.
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

//go:build linux || windows || darwin

package ledcmd

import (
	"context"

	"github.com/mattn/go-tty"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

func awaitNewline(ctx context.Context) error {
	term, err := tty.Open()
	if err != nil {
		return errors.Annotate(err, "opening terminal").Err()
	}
	defer term.Close()

	logging.Infof(ctx, "When finished, press <enter> here to isolate it.")
	_, err = term.ReadString()
	if err != nil {
		return errors.Annotate(err, "reading <enter>").Err()
	}
	return nil
}
