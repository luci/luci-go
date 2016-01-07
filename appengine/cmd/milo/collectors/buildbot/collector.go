// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/milo/model"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
)

// BuildExtractURL is the URL of Chrome Build extract, which caches buildbot
// json.
const BuildExtractURL = "https://chrome-build-extract.appspot.com"

const workPoolSizeMultiplier = 2

type changeJSON struct {
	Repository string `json:"repository"`
	Digest     string `json:"revision"`
}

type sourceJSON struct {
	Changes []changeJSON `json:"changes"`
}

type buildJSON struct {
	// Times elements are a float64 unix time that the build was started. It's
	// an array because buildbot :)
	Times       []float64  `json:"times"`
	Builder     string     `json:"builderName"`
	Master      string     `json:"masterName"`
	Error       string     `json:"error"`
	Logs        [][]string `json:"logs"`
	SourceStamp sourceJSON `json:"sourceStamp"`
}

func (b *buildJSON) toDS(c context.Context) *model.Build {
	revisions := make([]model.RevisionInfo, len(b.SourceStamp.Changes))
	parallel.FanOutIn(func(ch chan<- func() error) {
		for i, change := range b.SourceStamp.Changes {
			i := i
			change := change
			ch <- func() error {
				repo := model.GetRepository(c, change.Repository)
				rev, err := repo.GetRevision(c, change.Digest)
				if err != nil {
					// Just ignore the error, and put a revision key with a dummy
					// generation number for now. This will be over-written later, if
					// the revision gets put into the datastore and the backfiller is
					// run again.
					revisions[i] = model.RevisionInfo{
						Repository: change.Repository,
						Digest:     change.Digest,
						Generation: -1,
					}
					return nil
				}

				revisions[i] = model.RevisionInfo{
					Repository: rev.Repository.StringID(),
					Digest:     rev.Digest,
					Generation: rev.Generation,
				}
				return nil
			}
		}
	})

	return &model.Build{
		ExecutionTime: model.TimeID{time.Unix(int64(b.Times[0]), 0).UTC()},
		BuildRoot:     model.GetBuildRoot(c, b.Master, b.Builder).Key,
		BuildLogKey:   b.Logs[1][1],
		Revisions:     revisions,
	}
}

type builderJSON struct {
	Builds []int `json:"cachedBuilds"`
}

type masterJSON struct {
	Builders map[string]builderJSON `json:"builders"`
}

func getBuild(master, builder string, build int) (*buildJSON, error) {
	resp, err := http.Get(strings.Join(
		[]string{BuildExtractURL, "p", master, "builders", builder, "builds", fmt.Sprintf("%d", build)},
		"/") + "?json=1")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	doneBuild := &buildJSON{}
	err = dec.Decode(doneBuild)
	if err != nil {
		return nil, err
	}

	if doneBuild.Error != "" {
		return nil, fmt.Errorf("error while getting Build for master %s builder %s build number %d: %s", master, builder, build, doneBuild.Error)
	}
	return doneBuild, nil
}

// PopulateMaster puts the data for a master into the datastore.
//
// It hits CBE to grab the json, and then creates corresponding entities and
// puts them into the datastore. It assumes that a datastore impl exists in the
// supplied context.
func PopulateMaster(c context.Context, master string) error {
	masterJSON := &masterJSON{}
	resp, err := http.Get(strings.Join([]string{BuildExtractURL, "get_master", master}, "/"))
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(masterJSON)
	if err != nil {
		return err
	}

	buildsToPut := make(map[string][]*model.Build)
	log.Infof(c, "Getting builds for master %s\n", master)

	errors := parallel.WorkPool(workPoolSizeMultiplier*len(masterJSON.Builders), func(ch chan<- func() error) {
		for name, builder := range masterJSON.Builders {
			if len(builder.Builds) > 0 {
				buildsToPut[name] = make([]*model.Build, len(builder.Builds))

				for ind, builderNum := range builder.Builds {
					ind := ind
					builderNum := builderNum
					builderName := name
					ch <- func() error {
						res, err := getBuild(master, builderName, builderNum)
						if err != nil {
							log.Warningf(c, "got %s", err)
							return err
						}

						conv := res.toDS(c)
						buildsToPut[builderName][ind] = conv
						return nil
					}
				}
				log.Infof(c, "Queued builds for builder %s.", name)
			}
		}
	})

	// We ignore these warnings because, for some reason, CBE sometimes returns
	// 404s for some builds. So we ignore those, and just move on. They're
	// printed to the user above, so if something is really wrong all those
	// errors will show up.
	log.Warningf(c, "Done fetching builds. errors: %s.\n", errors)

	for name, builds := range buildsToPut {
		num := 0
		for _, b := range builds {
			if b != nil {
				num++
			}
		}

		if num == 0 {
			buildsToPut[name] = nil
			continue
		}

		newBuilds := make([]*model.Build, num)
		ind := 0
		for _, b := range builds {
			if b != nil {
				newBuilds[ind] = b
				ind++
			}
		}
		buildsToPut[name] = newBuilds
	}

	ds := datastore.Get(c)
	for builderName, toPut := range buildsToPut {
		log.Infof(c, "%d to put for builder %s\n", len(toPut), builderName)
		if toPut != nil {
			ds.PutMulti(toPut)
		}
	}
	return nil
}
