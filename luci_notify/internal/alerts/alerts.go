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

// Package alerts manages alert values in the database.
package alerts

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"
)

// Alert mirrors the structure of alert values in the database.
type Alert struct {
	// The key of the alert.
	// For now, this matches the key of the alert in Sheriff-o-Matic.
	// This may be revised in the future.
	AlertKey string
	// The bug number in Buganizer/IssueTracker.
	// 0 if the alert is not linked to a bug.
	Bug int64
	// The build number to consider this alert silenced until.
	// 0 if the alert is not silenced.
	SilenceUntil int64
	// The time the alert was last modified.
	// Used to control TTL of alert values.
	ModifyTime time.Time
}

// Validate validates an alert value.
// It ignores the ModifyTime fields.
// Reported field names are as they appear in the RPC documentation rather than the Go struct.
func Validate(alert *Alert) error {
	if err := validateBug(alert.Bug); err != nil {
		return errors.Annotate(err, "bug").Err()
	}
	if err := validateSilenceUntil(alert.SilenceUntil); err != nil {
		return errors.Annotate(err, "silence_until").Err()
	}
	return nil
}

func validateBug(bug int64) error {
	if bug < 0 {
		return errors.Reason("must be zero or positive").Err()
	}
	return nil
}

func validateSilenceUntil(silenceUntil int64) error {
	if silenceUntil < 0 {
		return errors.Reason("must be zero or positive").Err()
	}
	return nil
}

// Put sets the data for the alert in the Spanner Database.
// Note that this implies removing the entry from the database
// if there is no non-default information to keep a small table size.
// ModifyTime in the passed in alert will be ignored
// in favour of the commit time.
func Put(alert *Alert) (*spanner.Mutation, error) {
	if err := Validate(alert); err != nil {
		return nil, err
	}

	if alert.Bug == 0 && alert.SilenceUntil == 0 {
		return spanner.Delete("Alerts", spanner.Key{alert.AlertKey}), nil
	}
	row := map[string]any{
		"AlertKey":     alert.AlertKey,
		"Bug":          alert.Bug,
		"SilenceUntil": alert.SilenceUntil,
		"ModifyTime":   spanner.CommitTimestamp,
	}
	return spanner.InsertOrUpdateMap("Alerts", row), nil
}

// ReadBatch retrieves a batch of alerts from the database.  If no record exists for an
// alert in the database an alert struct with all fields other than the key set to zero values will be returned.
// Returned alerts are in the same order as the keys requested.
// At most 100 keys can be requested in a batch.
func ReadBatch(ctx context.Context, keys []string) ([]*Alert, error) {
	if len(keys) > 100 {
		return nil, errors.Reason("requested a batch of %d keys, the maximum size is 100", len(keys)).Err()
	}
	if len(keys) == 0 {
		// Nothing to do.
		return []*Alert{}, nil
	}

	stmt := spanner.NewStatement(`
		SELECT
			AlertKey,
			Bug,
			SilenceUntil,
			ModifyTime
		FROM Alerts
		WHERE
			AlertKey IN UNNEST(@keys)`)
	stmt.Params["keys"] = keys

	iter := span.Query(ctx, stmt)

	alertMap := map[string]*Alert{}
	if err := iter.Do(func(row *spanner.Row) error {
		alert, err := fromRow(row)
		if err != nil {
			return err
		}
		alertMap[alert.AlertKey] = alert
		return nil
	}); err != nil {
		return nil, errors.Annotate(err, "read batch of alerts").Err()
	}
	// Sort return value according to input value, and insert blank entries.
	alerts := []*Alert{}
	for _, key := range keys {
		alert, ok := alertMap[key]
		if !ok {
			alert = &Alert{
				AlertKey:   key,
				ModifyTime: time.Time{},
			}
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

func fromRow(row *spanner.Row) (*Alert, error) {
	alert := &Alert{}
	if err := row.Column(0, &alert.AlertKey); err != nil {
		return nil, errors.Annotate(err, "reading AlertKey column").Err()
	}
	if err := row.Column(1, &alert.Bug); err != nil {
		return nil, errors.Annotate(err, "reading Bug column").Err()
	}
	if err := row.Column(2, &alert.SilenceUntil); err != nil {
		return nil, errors.Annotate(err, "reading SilenceUntil column").Err()
	}
	if err := row.Column(3, &alert.ModifyTime); err != nil {
		return nil, errors.Annotate(err, "reading ModifyTime column").Err()
	}
	return alert, nil
}
