// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cloud

import (
	"google.golang.org/cloud/datastore"

	"golang.org/x/net/context"
)

// Use installs the cloud services implementation into the supplied Context.
//
// This includes:
//	- github.com/luci/gae/service/info
//	- github.com/luci/gae/service/datastore
//
// This is built around the ability to use cloud datastore.
func Use(c context.Context, client *datastore.Client) context.Context {
	cds := cloudDatastore{
		client: client,
	}

	return cds.use(useInfo(c))
}
