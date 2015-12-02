// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"fmt"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
)

// ViewQuestsLimit is a hard-coded limit on the number of Quests that will
// be returned in a single query.
//
// TODO(iannucci): make this configurable in the request
const ViewQuestsLimit = 100

// ViewQuestsReq is the request for the ViewQuests endpoint.
type ViewQuestsReq struct {
	WithAttempts bool
	From         string
	To           string
}

// Valid returns an error if this ViewQuestsReq is not in a valid state.
func (req *ViewQuestsReq) Valid() error {
	if req.To != "" && req.From != "" && req.To < req.From {
		return fmt.Errorf("inverted boundaries")
	}
	return nil
}

// ViewQuests allows the user to view a range of Quests by ID.
func (d *DungeonMaster) ViewQuests(c context.Context, req *ViewQuestsReq) (rsp *display.Data, err error) {
	if c, err = d.Use(c, MethodInfo["ViewQuests"]); err != nil {
		return
	}

	if err = req.Valid(); err != nil {
		return
	}

	ds := datastore.Get(c)
	q := datastore.NewQuery("Quest")
	if req.From != "" {
		q = q.Gte("__key__", ds.KeyForObj(&model.Quest{ID: req.From}))
	}
	if req.To != "" {
		q = q.Lt("__key__", ds.KeyForObj(&model.Quest{ID: req.To}))
	}
	q.Limit(ViewQuestsLimit + 1)

	quests := []*model.Quest{}
	if err = ds.GetAll(q, &quests); err != nil {
		return
	}

	rsp = &display.Data{}
	if len(quests) > ViewQuestsLimit {
		rsp.More = true
		quests = quests[:ViewQuestsLimit]
	}

	rsp.Quests = make([]*display.Quest, len(quests))
	for i, q := range quests {
		rsp.Quests[i] = q.ToDisplay()
	}

	if req.WithAttempts {
		rsltChan := make(chan *display.Attempt)
		waitChan := make(chan struct{})

		go func() {
			defer close(waitChan)
			for da := range rsltChan {
				rsp.Attempts.Merge(da)
			}
		}()

		err = func() error {
			defer close(rsltChan)
			return parallel.FanOutIn(func(ch chan<- func() error) {
				for _, qa := range rsp.Quests {
					id := qa.ID
					ch <- func() error {
						atmpts, err := (&model.Quest{ID: id}).GetAttempts(c)
						if err == nil {
							for _, a := range atmpts {
								rsltChan <- a.ToDisplay()
							}
						}
						return err
					}
				}
			})
		}()

		<-waitChan
	}

	return
}

func init() {
	MethodInfo["ViewQuests"] = &endpoints.MethodInfo{
		Name:       "quests.list",
		HTTPMethod: "GET",
		Path:       "quests",
		Desc:       "Recursively view all quests",
	}
}
