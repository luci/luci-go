// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package replication

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/urlfetch"
)

// linkToPrimary sends HttpRPC to the primary to register myself as a replica.
func linkToPrimary(c context.Context, ticket ServiceLinkTicket, initiatedBy string) error {
	headers := make(map[string]string)
	headers["Content-Type"] = "application/octet-stream"
	protocol := "https"
	if appengine.IsDevAppServer() {
		headers["X-Appengine-Inbound-Appid"] = appengine.AppID(c)
		protocol = "http"
	}

	linkReq := &ServiceLinkRequest{
		Ticket:      ticket.GetTicket(),
		ReplicaUrl:  proto.String(fmt.Sprintf("%s://%s", protocol, appengine.DefaultVersionHostname(c))),
		InitiatedBy: proto.String(initiatedBy),
	}
	buf, err := proto.Marshal(linkReq)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/auth_service/api/v1/internal/link_replica", ticket.GetPrimaryUrl()), bytes.NewReader(buf))
	if err != nil {
		return err
	}
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := urlfetch.Client(c)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("got status code %v; want 200", resp.StatusCode)
	}

	respBody := new(bytes.Buffer)
	respBody.ReadFrom(resp.Body)
	linkResp := &ServiceLinkResponse{}
	if err = proto.Unmarshal(respBody.Bytes(), linkResp); err != nil {
		return err
	}
	if linkResp.GetStatus() != ServiceLinkResponse_SUCCESS {
		return fmt.Errorf("Request to the primary failed with status %d", linkResp.GetStatus())
	}
	return nil
}
