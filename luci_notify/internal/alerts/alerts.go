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
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"regexp"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"
)

const (
	// AlertKeyExpression is a partial regular expression that validates alert keys.
	// For now this is quite loose to handle Sheriff-o-matic keys which contain builder name
	// strings, so could be any characters at all.
	// Example SOM key: chromium$!chrome$!ci$!linux-chromeos-chrome$!browser_tests on Ubuntu-22.04$!8754809345790718177
	// TODO: Once we switch away from sheriff-o-matic we should tighten this up.
	AlertKeyExpression = `[^/]+`
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
	// GerritCL is the CL number associated with this alert.
	// 0 means the alert is not associated with any GerritCL.
	GerritCL int64
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
		return errors.Fmt("bug: %w", err)
	}
	if err := validateGerritCL(alert.GerritCL); err != nil {
		return errors.Fmt("gerrit_cl: %w", err)
	}
	if err := validateSilenceUntil(alert.SilenceUntil); err != nil {
		return errors.Fmt("silence_until: %w", err)
	}
	return nil
}

func validateBug(bug int64) error {
	if bug < 0 {
		return errors.New("must be zero or positive")
	}
	return nil
}

func validateGerritCL(gerritCL int64) error {
	if gerritCL < 0 {
		return errors.New("must be zero or positive")
	}
	return nil
}

func validateSilenceUntil(silenceUntil int64) error {
	if silenceUntil < 0 {
		return errors.New("must be zero or positive")
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

	if alert.Bug == 0 && alert.GerritCL == 0 && alert.SilenceUntil == 0 {
		return spanner.Delete("Alerts", spanner.Key{alert.AlertKey}), nil
	}
	row := map[string]any{
		"AlertKey":     alert.AlertKey,
		"Bug":          alert.Bug,
		"GerritCL":     alert.GerritCL,
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
		return nil, errors.Fmt("requested a batch of %d keys, the maximum size is 100", len(keys))
	}
	if len(keys) == 0 {
		// Nothing to do.
		return []*Alert{}, nil
	}

	stmt := spanner.NewStatement(`
		SELECT
			AlertKey,
			Bug,
			GerritCL,
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
		return nil, errors.Fmt("read batch of alerts: %w", err)
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
		return nil, errors.Fmt("reading AlertKey column: %w", err)
	}
	var nullable spanner.NullInt64
	if err := row.Column(1, &nullable); err != nil {
		return nil, errors.Fmt("reading Bug column: %w", err)
	}
	alert.Bug = nullable.Int64
	if err := row.Column(2, &nullable); err != nil {
		return nil, errors.Fmt("reading GerritCL column: %w", err)
	}
	alert.GerritCL = nullable.Int64
	if err := row.Column(3, &nullable); err != nil {
		return nil, errors.Fmt("reading SilenceUntil column: %w", err)
	}
	alert.SilenceUntil = nullable.Int64
	if err := row.Column(4, &alert.ModifyTime); err != nil {
		return nil, errors.Fmt("reading ModifyTime column: %w", err)
	}
	return alert, nil
}

func (a *Alert) Etag() string {
	h := fnv.New32a()
	// Ignore errors here.
	_ = binary.Write(h, binary.LittleEndian, a.Bug)
	_ = binary.Write(h, binary.LittleEndian, a.GerritCL)
	_ = binary.Write(h, binary.LittleEndian, a.SilenceUntil)
	return fmt.Sprintf("W/\"%x\"", h.Sum32())
}

var alertNameRE = regexp.MustCompile(`^alerts/(` + AlertKeyExpression + `)$`)

// ParseAlertName parses an alert resource name into its constituent ID
// parts.
func ParseAlertName(name string) (key string, err error) {
	if name == "" {
		return "", errors.New("must be specified")
	}
	match := alertNameRE.FindStringSubmatch(name)
	if match == nil {
		return "", errors.Fmt("expected format: %s", alertNameRE)
	}
	return match[1], nil
}
