// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package replication

import (
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/common/auth/model"
)

// timestampToTime converts timestamp representation used in AuthDB protobuf to
// time.Time.
func timestampToTime(t int64) time.Time {
	return time.Unix(0, 0).Add(time.Duration(t) * time.Microsecond)
}

// timeToTimestamp converts time.Time to timestamp representation in AuthDB
// protobuf.
func timeToTimestamp(t time.Time) int64 {
	return (t.UnixNano() + 499) / 1000
}

// protoToAuthDBSnapshot converts AuthDB protobuf to model.AuthDBSnapshot.
func protoToAuthDBSnapshot(c context.Context, rev *AuthDBRevision, adb *AuthDB) model.AuthDBSnapshot {
	grps := adb.GetGroups()
	ipwls := adb.GetIpWhitelists()
	asmts := adb.GetIpWhitelistAssignments()

	snap := model.AuthDBSnapshot{
		ReplicationState: &model.AuthReplicationState{
			PrimaryID:         rev.GetPrimaryId(),
			AuthDBRev:         rev.GetAuthDbRev(),
			ModifiedTimestamp: timestampToTime(rev.GetModifiedTs()),
		},
		GlobalConfig: &model.AuthGlobalConfig{
			Key:                      model.RootKey(c),
			OAuthClientID:            adb.GetOauthClientId(),
			OAuthClientSecret:        adb.GetOauthClientSecret(),
			OAuthAdditionalClientIDs: adb.GetOauthAdditionalClientIds(),
		},
		Groups:       make([]*model.AuthGroup, 0, len(grps)),
		IPWhitelists: make([]*model.AuthIPWhitelist, 0, len(ipwls)),
		IPWhitelistAssignments: &model.AuthIPWhitelistAssignments{
			Key:         model.IPWhitelistAssignmentsKey(c),
			Assignments: make([]model.Assignment, 0, len(asmts)),
		},
	}

	for _, g := range grps {
		snap.Groups = append(snap.Groups, &model.AuthGroup{
			Key:               model.GroupKey(c, g.GetName()),
			Members:           g.GetMembers(),
			Globs:             g.GetGlobs(),
			Nested:            g.GetNested(),
			Description:       g.GetDescription(),
			CreatedTimestamp:  timestampToTime(g.GetCreatedTs()),
			CreatedBy:         g.GetCreatedBy(),
			ModifiedTimestamp: timestampToTime(g.GetModifiedTs()),
			ModifiedBy:        g.GetModifiedBy(),
		})
	}
	for _, wl := range ipwls {
		snap.IPWhitelists = append(snap.IPWhitelists, &model.AuthIPWhitelist{
			Key:               model.IPWhitelistKey(c, wl.GetName()),
			Subnets:           wl.GetSubnets(),
			Description:       wl.GetDescription(),
			CreatedTimestamp:  timestampToTime(wl.GetCreatedTs()),
			CreatedBy:         wl.GetCreatedBy(),
			ModifiedTimestamp: timestampToTime(wl.GetModifiedTs()),
			ModifiedBy:        wl.GetModifiedBy(),
		})
	}
	for _, a := range asmts {
		snap.IPWhitelistAssignments.Assignments = append(snap.IPWhitelistAssignments.Assignments, model.Assignment{
			Identity:         a.GetIdentity(),
			IPWhitelist:      a.GetIpWhitelist(),
			Comment:          a.GetComment(),
			CreatedTimestamp: timestampToTime(a.GetCreatedTs()),
			CreatedBy:        a.GetCreatedBy(),
		})
	}

	return snap
}

// ticketToReplicationState converts ServiceLinkTicket to AuthReplicationState.
func ticketToReplicationState(ticket ServiceLinkTicket) model.AuthReplicationState {
	return model.AuthReplicationState{
		PrimaryID:  ticket.GetPrimaryId(),
		PrimaryURL: ticket.GetPrimaryUrl(),
	}
}

// toAuthDBRevision converts AuthReplicationState to AuthDBRevision proto.
func toAuthDBRevision(rs *model.AuthReplicationState) *AuthDBRevision {
	return &AuthDBRevision{
		PrimaryId:  proto.String(rs.PrimaryID),
		AuthDbRev:  proto.Int64(rs.AuthDBRev),
		ModifiedTs: proto.Int64(timeToTimestamp(rs.ModifiedTimestamp)),
	}
}
