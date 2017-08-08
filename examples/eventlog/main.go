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

// eventlog is an example that demonstrates how to log to the eventlog service.
// It logs to a locally-run server. See go/localeventlog for more information.
package main

import (
	"fmt"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/eventlog"
	logpb "go.chromium.org/luci/common/eventlog/proto"
)

func main() {
	ctx := context.Background()
	c := eventlog.NewClient(ctx, "http://localhost:27910/log")

	event := c.NewLogEvent(ctx, eventlog.Point())

	// Log a build event. Other kinds of events may also be logged.
	event.InfraEvent.BuildEvent = &logpb.BuildEvent{ /* TODO:fill in contents */ }

	err := c.LogSync(ctx, event)
	if err != nil {
		fmt.Printf("logging: %v\n", err)
	}
}
