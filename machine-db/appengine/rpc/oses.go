// Copyright 2017 The LUCI Authors.
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

package rpc

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// ListOSes handles a request to retrieve operating systems.
func (*Service) ListOSes(c context.Context, req *crimson.ListOSesRequest) (*crimson.ListOSesResponse, error) {
	oses, err := listOSes(c, stringset.NewFromSlice(req.Names...))
	if err != nil {
		return nil, err
	}
	return &crimson.ListOSesResponse{
		Oses: oses,
	}, nil
}

// listOSes returns a slice of operating systems in the database.
func listOSes(c context.Context, names stringset.Set) ([]*crimson.OS, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT name, description
		FROM oses
	`)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch operating systems").Err()
	}
	defer rows.Close()

	var oses []*crimson.OS
	for rows.Next() {
		os := &crimson.OS{}
		if err = rows.Scan(&os.Name, &os.Description); err != nil {
			return nil, errors.Annotate(err, "failed to fetch operating system").Err()
		}
		if matches(os.Name, names) {
			oses = append(oses, os)
		}
	}
	return oses, nil
}
