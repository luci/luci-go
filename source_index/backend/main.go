// Copyright 2024 The LUCI Authors.
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
	"go.chromium.org/luci/common/errors"
	luciserver "go.chromium.org/luci/server"

	"go.chromium.org/luci/source_index/server"
)

// main implements the entrypoint for the backend service.
func main() {
	server.Main(func(srv *luciserver.Server) error {
		if err := server.RegisterCronHandlers(srv); err != nil {
			return errors.Annotate(err, "register cron handlers").Err()
		}
		if err := server.RegisterPubSubHandlers(srv); err != nil {
			return errors.Annotate(err, "register pubsub handlers").Err()
		}
		if err := server.RegisterTaskQueueHandlers(srv); err != nil {
			return errors.Annotate(err, "register task queue handlers").Err()
		}
		return nil
	})
}
