// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"fmt"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

const (
	// targetDataCenter is the value set on the "data_center" field in the
	// ts_mon.proto.Task message.
	targetDataCenter = "appengine"

	// pubsubProject and pubsubTopic specify which cloud pubsub endpoint to send
	// monitoring metrics to.  The appengine app using this library needs to have
	// the "Google Cloud Pub/Sub API" enabled in the cloud console.
	pubsubProject = "chrome-infra-mon-pubsub"
	pubsubTopic   = "monacq"

	// instanceNamespace is the namespace to use for datastore instances.
	instanceNamespace = "ts_mon_instance_namespace"

	instanceExpirationTimeout     = time.Duration(30 * time.Minute)
	instanceExpectedToHaveTaskNum = time.Duration(5 * time.Minute)
)

type instance struct {
	_kind       string    `gae:"$kind,Instance"`
	ID          string    `gae:"$id"`
	TaskNum     int       `gae:"task_num"`     // Field names should match Python
	LastUpdated time.Time `gae:"last_updated"` // implementation.
}

// instanceEntityID returns a string unique to this appengine module, version
// and instance, to be used as the datastore ID for an "instance" entity.
func instanceEntityID(c context.Context) string {
	i := info.Get(c)
	return fmt.Sprintf("%s.%s.%s", i.InstanceID(), i.VersionID(), i.ModuleName())
}

// getOrCreateInstanceEntity returns the instance entity for this appengine
// instance, adding a default one to the datastore if it doesn't exist.
func getOrCreateInstanceEntity(c context.Context) *instance {
	entity := instance{
		ID:          instanceEntityID(c),
		TaskNum:     -1,
		LastUpdated: clock.Get(c).Now().UTC(),
	}
	ds := datastore.Get(c)

	err := ds.RunInTransaction(func(c context.Context) error {
		switch err := ds.Get(&entity); err {
		case nil:
			return nil
		case datastore.ErrNoSuchEntity:
			// Insert it into datastore if it didn't exist.
			return ds.Put(&entity)
		default:
			return err
		}
	}, &datastore.TransactionOptions{})

	if err != nil {
		logging.Errorf(c, "getOrCreateInstanceEntity failed: %s", err)
	}
	return &entity
}
