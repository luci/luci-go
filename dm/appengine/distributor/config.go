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

package distributor

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/gae/service/info"
	tq "go.chromium.org/gae/service/taskqueue"
	"go.chromium.org/luci/common/gcloud/pubsub"
	dm "go.chromium.org/luci/dm/api/service/v1"
)

// Config represents the configuration for a single instance of a given
// distributor implementation at a given point in time (e.g. version).
type Config struct {
	// DMHost is the host for the DM API. This may be used by the distributor
	// implementation to pass to jobs so that they can call back into DM's api.
	DMHost string

	// Name is the name of this distributor configuration. This is always the
	// fully-resolved name of the configuration (i.e. aliases are dereferenced).
	Name string

	// Version is the version of the distributor configuration retrieved from
	// luci-config.
	Version string

	// Content is the actual parsed implementation-specific configuration.
	Content proto.Message
}

// EnqueueTask allows a Distributor to enqueue a TaskQueue task that will be
// handled by the Distributor's HandleTaskQueueTask method.
func (cfg *Config) EnqueueTask(c context.Context, tsk *tq.Task) error {
	tsk.Path = handlerPath(cfg.Name)
	return tq.Add(c, "", tsk)
}

// PrepareTopic returns a pubsub topic that notifications should be sent to, and
// is meant to be called from the D.Run method.
//
// It returns the full name of the topic and a token that will be used to route
// PubSub messages back to the Distributor. The publisher to the topic must be
// instructed to put the token into the 'auth_token' attribute of PubSub
// messages. DM will know how to route such messages to D.HandleNotification.
func (cfg *Config) PrepareTopic(c context.Context, eid *dm.Execution_ID) (topic pubsub.Topic, token string, err error) {
	topic = pubsub.NewTopic(info.TrimmedAppID(c), notifyTopicSuffix)
	if err := topic.Validate(); err != nil {
		panic(fmt.Errorf("failed to validate Topic %q: %s", topic, err))
	}
	token, err = encodeAuthToken(c, eid, cfg.Name)
	return
}
