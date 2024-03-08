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

// Package rpc contains the RPC handlers for the LUCI Notify service.
package rpc

import (
	"context"
	"fmt"
	"regexp"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/internal/alerts"
)

type alertsServer struct{}

var _ pb.AlertsServer = &alertsServer{}

// NewAlertsServer creates a new server to handle Alerts requests.
func NewAlertsServer() *pb.DecoratedAlerts {
	return &pb.DecoratedAlerts{
		Prelude:  checkAllowedPrelude,
		Service:  &alertsServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// toAlertProto converts an alerts.Alert value to a pb.Alert proto.
func toAlertProto(value *alerts.Alert) *pb.Alert {
	return &pb.Alert{
		Name:         fmt.Sprintf("alerts/%s", value.AlertKey),
		Bug:          value.Bug,
		SilenceUntil: value.SilenceUntil,
		ModifyTime:   timestamppb.New(value.ModifyTime),
		Etag:         value.Etag(),
	}
}

// BatchGetAlerts gets a number of alerts by name.
func (*alertsServer) BatchGetAlerts(ctx context.Context, request *pb.BatchGetAlertsRequest) (*pb.BatchGetAlertsResponse, error) {
	keys := []string{}
	for i, name := range request.Names {
		key, err := parseAlertName(name)
		if err != nil {
			return nil, invalidArgumentError(errors.Annotate(err, "name[%v]", i).Err())
		}
		keys = append(keys, key)
	}
	alerts, err := alerts.ReadBatch(span.Single(ctx), keys)
	if err != nil {
		return nil, errors.Annotate(err, "reading alerts").Err()
	}
	response := &pb.BatchGetAlertsResponse{}
	for _, alert := range alerts {
		response.Alerts = append(response.Alerts, toAlertProto(alert))
	}
	return response, nil
}

// BatchUpdateAlerts updates a number of alerts in a single transaction.
// Note that although AIP-134 specifies that Update should not succeed if the entity does not exist,
// we consider every possible alert to already exist and we are only backing it with a sparse array.
func (*alertsServer) BatchUpdateAlerts(ctx context.Context, request *pb.BatchUpdateAlertsRequest) (*pb.BatchUpdateAlertsResponse, error) {
	hasWriteAccess, err := auth.IsMember(ctx, luciNotifyWriteAccessGroup)
	if err != nil {
		return nil, errors.Annotate(err, "checking write group membership").Err()
	}
	// TODO: Once alerts are moved to LUCI Notify from SOM we need to do tighter ACL checks here,
	// i.e. check that the user has some permission to the builder that the alert is for.
	if !hasWriteAccess {
		if auth.CurrentIdentity(ctx).Kind() == identity.Anonymous {
			return nil, permissionDeniedError(errors.New("please log in before updating alerts"))
		}
		return nil, permissionDeniedError(errors.New("you do not have permission to update alerts"))
	}

	response := &pb.BatchUpdateAlertsResponse{}
	keys := []string{}
	mutations := []*spanner.Mutation{}
	for i, r := range request.Requests {
		key, err := parseAlertName(r.Alert.Name)
		if err != nil {
			return nil, invalidArgumentError(errors.Annotate(err, "alerts[%v]: name", i).Err())
		}
		keys = append(keys, key)
		a := &alerts.Alert{
			AlertKey:     key,
			Bug:          r.Alert.Bug,
			SilenceUntil: r.Alert.SilenceUntil,
		}
		m, err := alerts.Put(a)
		if err != nil {
			return nil, invalidArgumentError(errors.Annotate(err, "alerts[%v]", i).Err())
		}
		mutations = append(mutations, m)
		response.Alerts = append(response.Alerts, &pb.Alert{
			Name:         fmt.Sprintf("alerts/%s", a.AlertKey),
			Bug:          a.Bug,
			SilenceUntil: a.SilenceUntil,
			Etag:         a.Etag(),
		})
	}

	ts, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		currentAlerts, err := alerts.ReadBatch(ctx, keys)
		if err != nil {
			return errors.Annotate(err, "reading existing alert values").Err()
		}
		for i := 0; i < len(request.Requests); i++ {
			if request.Requests[i].Alert.Etag == "" {
				continue
			}
			if request.Requests[i].Alert.Etag != currentAlerts[i].Etag() {
				return abortedError(errors.New(fmt.Sprintf("etag does not match on alert[%v]", i)))
			}
		}
		span.BufferWrite(ctx, mutations...)
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "apply update alerts to spanner").Err()
	}

	for _, a := range response.Alerts {
		a.ModifyTime = timestamppb.New(ts)
	}
	return response, nil
}

var alertNameRE = regexp.MustCompile(`^alerts/(` + alerts.AlertKeyExpression + `)$`)

// parseAlertName parses an alert resource name into its constituent ID
// parts.
func parseAlertName(name string) (key string, err error) {
	if name == "" {
		return "", errors.Reason("must be specified").Err()
	}
	match := alertNameRE.FindStringSubmatch(name)
	if match == nil {
		return "", errors.Reason("expected format: %s", alertNameRE).Err()
	}
	return match[1], nil
}

// invalidArgumentError annotates err as having an invalid argument.
// The error message is shared with the requester as is.
//
// Note that this differs from FailedPrecondition. It indicates arguments
// that are problematic regardless of the state of the system
// (e.g., a malformed file name).
func invalidArgumentError(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "%s", err)
}

// permissionDeniedError annotates err as being denied (HTTP 403).
// The error message is shared with the requester as is.
func permissionDeniedError(err error) error {
	return appstatus.Attachf(err, codes.PermissionDenied, "%s", err)
}

// abortedError annotates err as being aborted.
// The error message is shared with the requester as is.
func abortedError(err error) error {
	return appstatus.Attachf(err, codes.Aborted, "%s", err)
}
