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

// Package teams encapsulates reading/writing teams entities to Spanner.
package teams

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"
)

const (
	// TeamIDExpression is a partial regular expression that validates team identifiers.
	TeamIDExpression = `[0-9a-f]{32}`
)

// NotExistsErr is returned when the requested object was not found in the database.
var NotExistsErr error = errors.New("team was not found")

// Team mirrors the structure of team values in the database.
type Team struct {
	// Unique identifier of the team.
	ID string
	// Time the team was created.
	CreateTime time.Time
}

// Validate validates a team value.
func Validate(team *Team) error {
	if err := validateID(team.ID); err != nil {
		return errors.Annotate(err, "id").Err()
	}
	return nil
}

var teamIDRE = regexp.MustCompile(`^` + TeamIDExpression + `$`)

func validateID(id string) error {
	if id == "" {
		return errors.Reason("must be specified").Err()
	}
	if !teamIDRE.MatchString(id) {
		return errors.Reason("expected format: %s", teamIDRE).Err()
	}
	return nil
}

// Create creates a team entry in the Spanner Database.
// Must be called with an active RW transaction in the context.
// CreateTime in the passed in team will be ignored
// in favour of the commit time.
func Create(team *Team) (*spanner.Mutation, error) {
	if err := Validate(team); err != nil {
		return nil, err
	}

	row := map[string]any{
		"Id":         team.ID,
		"CreateTime": spanner.CommitTimestamp,
	}
	return spanner.InsertOrUpdateMap("Teams", row), nil
}

// Read retrieves a status update from the database given the exact time the status update was made.
func Read(ctx context.Context, ID string) (*Team, error) {
	row, err := span.ReadRow(ctx, "Teams", spanner.Key{ID}, []string{"ID", "CreateTime"})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return nil, NotExistsErr
		}
		return nil, errors.Annotate(err, "get status").Err()
	}
	return fromRow(row)
}

func fromRow(row *spanner.Row) (*Team, error) {
	team := &Team{}
	if err := row.Column(0, &team.ID); err != nil {
		return nil, errors.Annotate(err, "reading ID column").Err()
	}
	if err := row.Column(1, &team.CreateTime); err != nil {
		return nil, errors.Annotate(err, "reading CreateTime column").Err()
	}
	return team, nil
}

// GenerateID returns a random 128-bit team ID, encoded as
// 32 lowercase hexadecimal characters.
func GenerateID() (string, error) {
	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(randomBytes), nil
}
