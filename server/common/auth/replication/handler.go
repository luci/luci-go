// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// These are reference implementations of HTTP handlers.

package replication

import (
	"encoding/base64"
	"io/ioutil"
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/log"

	"github.com/luci/luci-go/server/common/auth"
	"github.com/luci/luci-go/server/common/auth/model"
)

// PushHandler handles AuthDB push from Primary.
type PushHandler struct {
}

func pushResponse(w http.ResponseWriter, resp *ReplicationPushResponse) {
	body, err := proto.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(body)
}

func primaryId(c context.Context) (string, error) {
	rs := &model.AuthReplicationState{}
	if err := model.GetReplicationState(c, rs); err != nil {
		return "", err
	}
	return rs.PrimaryID, nil
}

// ServeHTTP is an HTTP RPC handler for ReplicationPushRequest.
//
// Note that request should be signed, and should be sent with a signature.
func (rh *PushHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if r.Method != "POST" {
		log.Errorf(c, "method does not match we want: %v", r.Method)
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf(c, "failed to read request body: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req := &ReplicationPushRequest{}
	if err := proto.Unmarshal(body, req); err != nil {
		// The request might not be expected HttpRPC.
		// It should be natural to return HTTP error response.
		log.Errorf(c, "failed to unmarshal request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	prid, err := primaryId(c)
	if err != nil {
		log.Errorf(c, "failed to get primary id: %v", err)
		http.Error(w, "internal failure", http.StatusInternalServerError)
		return
	}
	if prid == "" {
		log.Errorf(c, "empty primary id. might not be replica?")
		// could be called before service is switched to replica mode.
		pushResponse(w, &ReplicationPushResponse{
			Status:          ReplicationPushResponse_TRANSIENT_ERROR.Enum(),
			ErrorCode:       ReplicationPushResponse_NOT_A_REPLICA.Enum(),
			AuthCodeVersion: proto.String(auth.Version),
		})
		return
	}
	inboundAppID := r.Header.Get("X-Appengine-Inbound-AppId")
	if inboundAppID != prid {
		log.Errorf(c, "X-Appengine-Inbound-AppId (%v) is not expected primary ID (%v)", inboundAppID, prid)
		pushResponse(w, &ReplicationPushResponse{
			Status:          ReplicationPushResponse_FATAL_ERROR.Enum(),
			ErrorCode:       ReplicationPushResponse_FORBIDDEN.Enum(),
			AuthCodeVersion: proto.String(auth.Version),
		})
		return
	}
	if req.GetRevision().GetPrimaryId() != prid {
		log.Errorf(c, "primary id in request (%v) is not expected primary ID (%v)", req.GetRevision().GetPrimaryId(), prid)
		pushResponse(w, &ReplicationPushResponse{
			Status:          ReplicationPushResponse_FATAL_ERROR.Enum(),
			ErrorCode:       ReplicationPushResponse_FORBIDDEN.Enum(),
			AuthCodeVersion: proto.String(auth.Version),
		})
		return
	}
	log.Infof(c, "Received AuthDB push: rev %v", req.GetRevision().GetAuthDbRev())
	log.Infof(c, "primary auth code version %v", req.GetAuthCodeVersion())

	sigKey := r.Header.Get("X-AuthDB-SigKey-v1")
	b64Sig := r.Header.Get("X-AuthDB-SigVal-v1")
	if sigKey == "" || b64Sig == "" {
		log.Errorf(c, "signature key name or signature is missing. key=%v, sig=%v", sigKey, b64Sig)
		pushResponse(w, &ReplicationPushResponse{
			Status:          ReplicationPushResponse_FATAL_ERROR.Enum(),
			ErrorCode:       ReplicationPushResponse_MISSING_SIGNATURE.Enum(),
			AuthCodeVersion: proto.String(auth.Version),
		})
		return
	}
	sig, err := base64.StdEncoding.DecodeString(b64Sig)
	if err != nil {
		log.Errorf(c, "decode failed %v %v", b64Sig, err)
		pushResponse(w, &ReplicationPushResponse{
			Status:          ReplicationPushResponse_FATAL_ERROR.Enum(),
			ErrorCode:       ReplicationPushResponse_BAD_SIGNATURE.Enum(),
			AuthCodeVersion: proto.String(auth.Version),
		})
		return
	}
	if err := verifySignature(c, sigKey, body, sig); err != nil {
		log.Errorf(c, "verify failed %v", err)
		pushResponse(w, &ReplicationPushResponse{
			Status:          ReplicationPushResponse_FATAL_ERROR.Enum(),
			ErrorCode:       ReplicationPushResponse_BAD_SIGNATURE.Enum(),
			AuthCodeVersion: proto.String(auth.Version),
		})
		return
	}

	applied, rev, err := PushAuthDB(c, req.GetRevision(), req.GetAuthDb())
	if err != nil {
		log.Errorf(c, "failed to push AuthDB %v", err)
		http.Error(w, "failed to update AuthDB", http.StatusInternalServerError)
		return
	}
	status := ReplicationPushResponse_SKIPPED.Enum()
	if applied {
		status = ReplicationPushResponse_APPLIED.Enum()
	}
	pushResponse(w, &ReplicationPushResponse{
		Status:          status,
		CurrentRevision: rev,
		AuthCodeVersion: proto.String(auth.Version),
	})
}
