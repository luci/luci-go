// Copyright 2018 The LUCI Authors.
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

package cloudlogger

import (
	cloudLogging "cloud.google.com/go/logging"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/logging"
)

// Use registers the cloud logger into the context.  Use takes two cloudLogging.Loggers,
// main and lines.  Main is required, but lines is optional.
// * main is specified, lines is nil: All log entries will be treated the same
// * both main and lines are specified: Logs with RequestKey in fields will go to main,
//   and all other logs will go to lines.
func Use(c context.Context, main, lines *cloudLogging.Logger) context.Context {
	return logging.SetFactory(c, GetFactory(main, lines))
}
