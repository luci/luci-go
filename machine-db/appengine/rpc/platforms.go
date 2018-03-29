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

	"github.com/Masterminds/squirrel"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// ListPlatforms handles a request to retrieve platforms.
func (*Service) ListPlatforms(c context.Context, req *crimson.ListPlatformsRequest) (*crimson.ListPlatformsResponse, error) {
	platforms, err := listPlatforms(c, req)
	if err != nil {
		return nil, err
	}
	return &crimson.ListPlatformsResponse{
		Platforms: platforms,
	}, nil
}

// listPlatforms returns a slice of platforms in the database.
func listPlatforms(c context.Context, req *crimson.ListPlatformsRequest) ([]*crimson.Platform, error) {
	stmt := squirrel.Select("name", "description", "manufacturer").From("platforms")
	stmt = selectInString(stmt, "names", req.Names)
	stmt = selectInString(stmt, "manufacturer", req.Manufacturers)
	query, args, err := stmt.ToSql()
	if err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to generate statement").Err())
	}

	db := database.Get(c)
	rows, err := db.QueryContext(c, query, args...)
	if err != nil {
		return nil, internalError(c, errors.Annotate(err, "failed to fetch platforms").Err())
	}
	defer rows.Close()

	var platforms []*crimson.Platform
	for rows.Next() {
		p := &crimson.Platform{}
		if err = rows.Scan(&p.Name, &p.Description, &p.Manufacturer); err != nil {
			return nil, internalError(c, errors.Annotate(err, "failed to fetch platform").Err())
		}
		platforms = append(platforms, p)
	}
	return platforms, nil
}
