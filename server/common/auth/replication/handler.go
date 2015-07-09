// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// These are reference implementations of HTTP handlers.

package replication

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/user"

	"github.com/luci/luci-go/server/common/auth"
	"github.com/luci/luci-go/server/common/auth/model"
	"github.com/luci/luci-go/server/common/auth/xsrf"
)

// PushHandler handles AuthDB push from Primary.
//
// This handler must be used in this manner:
//
//     http.Handler(replication.PushHandlerPath, replication.PushHandler{})
//
// You must use this path.
type PushHandler struct{}

// PushHandlerPath is the push handler's path.
const PushHandlerPath = "/auth/api/v1/internal/replication"

func pushResponse(w http.ResponseWriter, resp *ReplicationPushResponse) {
	body, err := proto.Marshal(resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(body)
}

func primaryID(c context.Context) (string, error) {
	rs := &model.AuthReplicationState{}
	if err := model.GetReplicationState(c, rs); err != nil {
		return "", err
	}
	return rs.PrimaryID, nil
}

// ServeHTTP is an HTTP RPC handler for ReplicationPushRequest.
//
// Note that request should be signed, and should be sent with a signature.
func (rh PushHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	prid, err := primaryID(c)
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
	if err := ValidateAuthDB(req.GetAuthDb()); err != nil {
		log.Errorf(c, "invalid AuthDB %v", err)
		pushResponse(w, &ReplicationPushResponse{
			Status:          ReplicationPushResponse_FATAL_ERROR.Enum(),
			ErrorCode:       ReplicationPushResponse_BAD_REQUEST.Enum(),
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

var (
	linkingTemplate = template.Must(template.New("linking").Parse(`
<!DOCTYPE html>
<html>
<!-- Copyright 2015 The Chromium Authors. All rights reserved.
Use of this source code is governed by the Apache v2.0 license that can be
found in the LICENSE file. -->
<head>
  <title>Switch</title>
  <meta http-equiv="Content-type" content="text/html; charset=UTF-8">
</head>

<body>
  <div class="container">
   <h3>Use {{.PrimaryID}} as a primary authentication service?</h3>
   <p>After the switch all user group management (UI and REST API) will be
   performed on <a href="{{.PrimaryURL}}">{{.PrimaryID}}</a>.
   </p>
   <ul>
     <li>Proposed by {{.GeneratedBy}}</li>
     <li>Primary service app id is {{.PrimaryID}}</li>
     <li>Primary service URL is <a href="{{.PrimaryURL}}">{{.PrimaryURL}}</a></li>
   </ul>
   <p><b>All current groups will be permanently deleted. There's no undo.</b></p>
   <hr>
   <p>
     <a class="btn btn-danger" href="#" onclick="javascript:submit(event);">
       Switch
     </a>
   </p>

   <form name="token" method="POST">
     <input type="hidden" name="xsrf_token" value="{{.XSRFToken}}" />
   </form>

   <script type="text/javascript">
   function submit(event) {
     document.token.submit();
     event.preventDefault();
     return false;
   }
   </script>
  </div>
</body>

</html>
`))
	linkingDoneTemplate = template.Must(template.New("done").Parse(`
<!DOCTYPE html>
<html>
<!-- Copyright 2015 The Chromium Authors. All rights reserved.
Use of this source code is governed by the Apache v2.0 license that can be
found in the LICENSE file. -->
<head>
  <title>Switch</title>
  <meta http-equiv="Content-type" content="text/html; charset=UTF-8">
</head>

<body>
  <div class="container">
  {{ if .Success }}
  <h3>Success!</h3>
  <p>The service is now configured to use
  <a href="{{.PrimaryURL}}">{{.PrimaryID}}</a> as a primary authentication and
  authorization service.
  </p>
  <p>
  The change may take several minutes to propagate.
  </p>
  {{ else }}
  <h3>Failed to switch the primary authentication service.</h3>
  <p>{{.ErrorMsg}}</p>
  {{ end }}
  <hr>

  </div>
</body>

</html>
`))
)

type linkingData struct {
	PrimaryID   string
	PrimaryURL  string
	GeneratedBy string
	XSRFToken   string
}

type linkingDoneData struct {
	Success    bool
	PrimaryID  string
	PrimaryURL string
	ErrorMsg   string
}

// LinkToPrimaryHandler confirms Primary <-> Replica linking request.
//
// This is a reference implementation.  Users can implement their own.
//
// However, even if you implement your own LinkToPriayaryHandler,
// you must use LinkToPrimaryHandlerPath as the LinkToPrimaryHandler path.
// For example, if you use the reference implementation, code would be:
//
//   http.Handle(replication.LinkToPrimaryHandlerPath, replication.LinkToPrimaryHandler{})
//
// You must use this path.
type LinkToPrimaryHandler struct{}

// LinkToPrimaryHandlerPath is a path for LinkToPrimaryHandler.
const LinkToPrimaryHandlerPath = "/auth/link"

// ServeHTTP handles Primary <-> Replica linking request.
func (lp LinkToPrimaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !user.IsAdmin(c) {
		url, err := user.LoginURL(c, r.URL.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
		return
	}
	t := r.FormValue("t")
	ticket, err := DecodeLinkTicket(t)
	if err != nil {
		log.Errorf(c, "failed to decode: %v", r.Method)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	action := fmt.Sprintf("LinkToPrimary:%s", t)
	switch r.Method {
	case "GET":
		token, err := xsrf.Generate(c, user.Current(c).String(), action)
		if err != nil {
			log.Errorf(c, "failed to create xsrf token: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		linkingTemplate.Execute(w, linkingData{
			PrimaryID:   ticket.GetPrimaryId(),
			PrimaryURL:  ticket.GetPrimaryUrl(),
			GeneratedBy: ticket.GetGeneratedBy(),
			XSRFToken:   token,
		})
	case "POST":
		err := xsrf.Check(c, r.FormValue("xsrf_token"), user.Current(c).String(), action)
		if err != nil {
			log.Errorf(c, "invalid xsrf token: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		err = BecomeReplica(c, ticket, user.Current(c).String())
		if err != nil {
			log.Errorf(c, "failed to be replica: %v", err)
			linkingDoneTemplate.Execute(w, linkingDoneData{
				Success:  false,
				ErrorMsg: err.Error(),
			})
			return
		}
		linkingDoneTemplate.Execute(w, linkingDoneData{
			Success:    true,
			PrimaryID:  ticket.GetPrimaryId(),
			PrimaryURL: ticket.GetPrimaryUrl(),
		})
	default:
		log.Errorf(c, "method does not match we want: %v", r.Method)
		w.Header().Set("Allow", "GET,POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}
