// Copyright 2025 The LUCI Authors.
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

// Package alertgroups contains functions for reading and writing AlertGroups
// from Spanner.
package alertgroups

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"regexp"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/luci_notify/internal/alerts"
	"go.chromium.org/luci/server/span"
)

const (
	// RotationIDExpression is a partial regular expression that validates rotation identifiers.
	RotationIdExpression = `[a-z0-9-]+`
	// AlertGroupIDExpression is a partial regular expression that validates alert group identifiers.
	AlertGroupIdExpression = `[a-z0-9-]+`
)

// Alert mirrors the structure of alert values in the database.
type AlertGroup struct {
	// The rotation the alert group belongs to.
	Rotation string
	// The ID of the alert group.
	AlertGroupId string
	// The human-readable name of the alert group.
	DisplayName string
	// A message describing the status of the alert group.
	StatusMessage string
	// The keys of the alerts in this group.
	AlertKeys []string
	// The bugs associated with this alert group.
	Bugs []int64
	// The identity of the user who last updated the alert group.
	UpdatedBy string
	// The time the alert group was last updated.
	// Output only.
	UpdateTime time.Time
}

// Validate validates an alert value.
// It ignores the UpdateTime field.
// Reported field names are as they appear in the RPC documentation rather than the Go struct.
func Validate(alert *AlertGroup) error {
	if err := validateRotation(alert.Rotation); err != nil {
		return errors.Fmt("rotation: %w", err)
	}
	if err := validateAlertGroupId(alert.AlertGroupId); err != nil {
		return errors.Fmt("alert_group_id: %w", err)
	}
	if err := validateDisplayName(alert.DisplayName); err != nil {
		return errors.Fmt("display_name: %w", err)
	}
	if err := validateStatusMessage(alert.StatusMessage); err != nil {
		return errors.Fmt("status_message: %w", err)
	}
	for i, key := range alert.AlertKeys {
		if err := validateAlertKey(key); err != nil {
			return errors.Fmt("alert_keys[%d]: %w", i, err)
		}
	}
	for i, bug := range alert.Bugs {
		if err := validateBug(bug); err != nil {
			return errors.Fmt("bugs[%d]: %w", i, err)
		}
	}
	if err := validateUpdatedBy(alert.UpdatedBy); err != nil {
		return errors.Fmt("updated_by: %w", err)
	}
	return nil
}

var rotationIDRegex = regexp.MustCompile(`^` + RotationIdExpression + `$`)

func validateRotation(rotation string) error {
	if rotation == "" {
		return errors.New("must not be empty")
	}
	if !rotationIDRegex.MatchString(rotation) {
		return errors.New("must contain only lowercase letters, numbers, and dashes")
	}
	return nil
}

var alertGroupIDRegex = regexp.MustCompile(`^` + AlertGroupIdExpression + `$`)

func validateAlertGroupId(alertGroupId string) error {
	if alertGroupId == "" {
		return errors.New("must not be empty")
	}
	if !alertGroupIDRegex.MatchString(alertGroupId) {
		return errors.New("must contain only lowercase letters, numbers, and dashes")
	}
	return nil
}

func validateDisplayName(displayName string) error {
	if displayName == "" {
		return errors.New("must not be empty")
	}
	if len(displayName) > 1024 {
		return errors.New("must not be longer than 1024 characters")
	}
	return nil
}

func validateStatusMessage(statusMessage string) error {
	if len(statusMessage) > 1024*1024 {
		return errors.New("must not be longer than 1048576 bytes")
	}
	return nil
}

func validateAlertKey(alertKey string) error {
	if _, err := alerts.ParseAlertName(alertKey); err != nil {
		return err
	}
	return nil
}

func validateBug(bug int64) error {
	if bug < 0 {
		return errors.New("must be zero or positive")
	}
	return nil
}

func validateUpdatedBy(updatedBy string) error {
	if updatedBy == "" {
		return errors.New("must not be empty")
	}
	if len(updatedBy) > 256 {
		return errors.New("must not be longer than 256 characters")
	}
	return nil
}

// Create inserts the alert group in to the Spanner Database.
// UpdateTime in the passed in alert will be ignored
// in favour of the commit time.
func Create(alertGroup *AlertGroup) (*spanner.Mutation, error) {
	if err := Validate(alertGroup); err != nil {
		return nil, err
	}

	row := map[string]any{
		"Rotation":      alertGroup.Rotation,
		"AlertGroupId":  alertGroup.AlertGroupId,
		"DisplayName":   alertGroup.DisplayName,
		"StatusMessage": alertGroup.StatusMessage,
		"AlertKeys":     alertGroup.AlertKeys,
		"Bugs":          alertGroup.Bugs,
		"UpdatedBy":     alertGroup.UpdatedBy,
		"UpdateTime":    spanner.CommitTimestamp,
	}
	return spanner.InsertMap("AlertGroups", row), nil
}

// Update updates the alert group in the Spanner Database.
// UpdateTime in the passed in alert will be ignored
// in favour of the commit time.
func Update(alertGroup *AlertGroup) (*spanner.Mutation, error) {
	if err := Validate(alertGroup); err != nil {
		return nil, err
	}

	row := map[string]any{
		"Rotation":      alertGroup.Rotation,
		"AlertGroupId":  alertGroup.AlertGroupId,
		"DisplayName":   alertGroup.DisplayName,
		"StatusMessage": alertGroup.StatusMessage,
		"AlertKeys":     alertGroup.AlertKeys,
		"Bugs":          alertGroup.Bugs,
		"UpdatedBy":     alertGroup.UpdatedBy,
		"UpdateTime":    spanner.CommitTimestamp,
	}
	return spanner.UpdateMap("AlertGroups", row), nil
}

// Read reads a single alert group from Spanner.
// If the alert group is not found, it returns an error with code codes.NotFound.
func Read(ctx context.Context, rotationId, alertGroupId string) (*AlertGroup, error) {
	stmt := spanner.NewStatement(`
		SELECT
			Rotation, AlertGroupId, DisplayName, StatusMessage, AlertKeys, Bugs, UpdateTime, UpdatedBy
		FROM AlertGroups
		WHERE Rotation = @rotation AND AlertGroupId = @id
	`)
	stmt.Params["rotation"] = rotationId
	stmt.Params["id"] = alertGroupId

	var g *AlertGroup
	it := span.Query(ctx, stmt)
	err := it.Do(func(r *spanner.Row) error {
		var err error
		g, err = fromRow(r)
		return err
	})
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, appstatus.Errorf(codes.NotFound, "alert group /rotations/%s/alertGroups/%s not found", rotationId, alertGroupId)
	}
	return g, nil
}

func fromRow(row *spanner.Row) (*AlertGroup, error) {
	alertGroup := &AlertGroup{}
	if err := row.Column(0, &alertGroup.Rotation); err != nil {
		return nil, errors.Fmt("reading Rotation column: %w", err)
	}
	if err := row.Column(1, &alertGroup.AlertGroupId); err != nil {
		return nil, errors.Fmt("reading AlertGroupId column: %w", err)
	}
	var nullString spanner.NullString
	if err := row.Column(2, &nullString); err != nil {
		return nil, errors.Fmt("reading DisplayName column: %w", err)
	}
	alertGroup.DisplayName = nullString.StringVal
	if err := row.Column(3, &nullString); err != nil {
		return nil, errors.Fmt("reading StatusMessage column: %w", err)
	}
	alertGroup.StatusMessage = nullString.StringVal
	if err := row.Column(4, &alertGroup.AlertKeys); err != nil {
		return nil, errors.Fmt("reading AlertKeys column: %w", err)
	}
	if err := row.Column(5, &alertGroup.Bugs); err != nil {
		return nil, errors.Fmt("reading Bugs column: %w", err)
	}
	if err := row.Column(6, &alertGroup.UpdateTime); err != nil {
		return nil, errors.Fmt("reading UpdateTime column: %w", err)
	}
	if err := row.Column(7, &alertGroup.UpdatedBy); err != nil {
		return nil, errors.Fmt("reading UpdatedBy column: %w", err)
	}
	return alertGroup, nil
}

// List reads a page of alert groups from Spanner.
func List(ctx context.Context, rotationId string, pageSize int, pageToken string) ([]*AlertGroup, string, error) {
	st := spanner.NewStatement(`
		SELECT
			Rotation, AlertGroupId, DisplayName, StatusMessage, AlertKeys, Bugs, UpdateTime, UpdatedBy
		FROM AlertGroups
		WHERE Rotation = @rotation AND AlertGroupId > @pageToken
		ORDER BY AlertGroupId
		LIMIT @pageSize
	`)
	st.Params["rotation"] = rotationId
	st.Params["pageSize"] = pageSize
	st.Params["pageToken"] = pageToken

	var groups []*AlertGroup
	it := span.Query(ctx, st)
	err := it.Do(func(r *spanner.Row) error {
		g, err := fromRow(r)
		if err != nil {
			return err
		}
		groups = append(groups, g)
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	nextPageToken := ""
	if len(groups) == pageSize {
		nextPageToken = groups[len(groups)-1].AlertGroupId
	}

	return groups, nextPageToken, nil
}

// Delete deletes an alert group from Spanner.
func Delete(ctx context.Context, rotationId, alertGroupId string) *spanner.Mutation {
	return spanner.Delete("AlertGroups", spanner.Key{rotationId, alertGroupId})
}

func (a *AlertGroup) Etag() string {
	h := fnv.New32a()
	// Ignore errors here.
	_, _ = h.Write([]byte(a.DisplayName))
	_, _ = h.Write([]byte(a.StatusMessage))
	for _, key := range a.AlertKeys {
		_, _ = h.Write([]byte(key))
	}
	for _, bug := range a.Bugs {
		_ = binary.Write(h, binary.LittleEndian, bug)
	}
	return fmt.Sprintf("W/\"%x\"", h.Sum32())
}

var rotationNameRE = regexp.MustCompile(`^rotations/(` + RotationIdExpression + `)$`)

func ParseRotationName(name string) (string, error) {
	if name == "" {
		return "", errors.New("must be specified")
	}
	match := rotationNameRE.FindStringSubmatch(name)
	if match == nil {
		return "", errors.Fmt("expected format: %s", rotationNameRE)
	}
	return match[1], nil
}

var alertGroupNameRE = regexp.MustCompile(`^rotations/(` + RotationIdExpression + `)/alertGroups/(` + AlertGroupIdExpression + `)$`)

func ParseAlertGroupName(name string) (string, string, error) {
	if name == "" {
		return "", "", errors.New("must be specified")
	}
	match := alertGroupNameRE.FindStringSubmatch(name)
	if match == nil {
		return "", "", errors.Fmt("expected format: %s", alertGroupNameRE)
	}
	return match[1], match[2], nil
}
