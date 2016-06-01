// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
	UnixTime   int64  `json:"when"`
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
	Number      int        `json:"number"`
	Text        []string   `json:"text"`
}

func getRevForChange(c context.Context, change changeJSON) model.RevisionInfo {
	repo := model.GetRepository(c, change.Repository)

	generation := -1
	if rev, err := repo.GetRevision(c, change.Digest); err == nil {
		generation = rev.Generation
	}

	return model.RevisionInfo{
		Repository: change.Repository,
		Digest:     change.Digest,
		Generation: generation,
	}
}

func (b *buildJSON) toDS(c context.Context) *model.Build {
	perRepo := make(map[string]*changeJSON)
	for _, change := range b.SourceStamp.Changes {
		current := perRepo[change.Repository]
		if current != nil {
			if current.UnixTime < change.UnixTime {
				perRepo[change.Repository] = &change
			}
		} else {
			perRepo[change.Repository] = &change
		}
	}

	revisions := make([]model.RevisionInfo, 0, len(perRepo))
	for _, change := range perRepo {
		revisions = append(revisions, getRevForChange(c, *change))
	}

	logKey := ""
	if len(b.Logs) > 1 && len(b.Logs[1]) > 1 {
		logKey = b.Logs[1][1]
	} else {
		log.Warningf(c, "build %d on %s master %s had weird logs\n", b.Number, b.Builder, b.Master)
	}

	userStatus := "UNKNOWN"
	if len(b.Text) < 2 {
		log.Warningf(c, "build %d on %s master %s had strange text\n", b.Number, b.Builder, b.Master)
	} else {
		if b.Text[0] == "exception" {
			userStatus = "EXCEPTION"
		} else if b.Text[0] == "failed" {
			userStatus = "FAILURE"
		} else if b.Text[1] == "successful" {
			userStatus = "SUCCESS"
		}
		if userStatus == "UNKNOWN" {
			log.Warningf(c, "build %d on %s master %s had unknown text %v\n", b.Number, b.Builder, b.Master, b.Text)
		}
	}

	return &model.Build{
		ExecutionTime: model.TimeID{time.Unix(int64(b.Times[0]), 0).UTC()},
		BuildRoot:     model.GetBuildRoot(c, b.Master, b.Builder).Key,
		BuildLogKey:   logKey,
		Revisions:     revisions,
		UserStatus:    userStatus,
	}
}

type builderJSON struct {
	Builds []int `json:"cachedBuilds"`
}

type masterJSON struct {
	Builders map[string]builderJSON `json:"builders"`
}

func getBuild(master, builder string, build int, useCBE bool) (*buildJSON, error) {
	resp := (*http.Response)(nil)
	err := (error)(nil)
	if useCBE {
		resp, err = http.Get(strings.Join(
			[]string{BuildExtractURL, "p", master, "builders", builder, "builds", fmt.Sprintf("%d", build)},
			"/") + "?json=1")
	} else {
		resp, err = http.Get(strings.Join(
			[]string{"https://build.chromium.org", "p", master, "json", "builders", builder, "builds", fmt.Sprintf("%d", build)},
			"/"))
	}

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
func PopulateMaster(c context.Context, master string, dryRun, buildbotFallback bool) error {
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
		for builderName, builder := range masterJSON.Builders {
			if len(builder.Builds) > 0 {
				buildsToPut[builderName] = make([]*model.Build, len(builder.Builds))

				for ind, builderNum := range builder.Builds {
					ind := ind
					builderNum := builderNum
					builderName := builderName

					ch <- func() error {
						res, err := getBuild(master, builderName, builderNum, true)
						if err != nil {
							if !buildbotFallback {
								log.Warningf(c, "got %s, giving up", err)
								return err
							}

							log.Warningf(c, "got %s, retrying without CBE", err)
							res, err = getBuild(master, builderName, builderNum, false)

							if err != nil {
								log.Warningf(c, "got %s, giving up", err)
								return err
							}
						}

						conv := res.toDS(c)
						buildsToPut[builderName][ind] = conv
						return nil
					}
				}
				log.Infof(c, "Queued builds for builder %s.", builderName)
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
			if dryRun {
				log.Infof(c, "dry run, not putting any modifications")
			} else {
				ds.PutMulti(toPut)
			}
		}
	}
	return nil
}
