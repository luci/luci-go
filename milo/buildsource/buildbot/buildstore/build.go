// Copyright 2017 The LUCI Authors.
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

package buildstore

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/common"
)

// ErrTooBig indicates that entity was not saved because it was too large to store.
var ErrTooBig = errors.New("entity was not saved because it was too large to store.")

// maxDataSize is maximum number of bytes for "data" field in build or master
// entities.
// Datastore has a max size of 1MB. If the blob is over 9.5MB, it probably
// won't fit after accounting for overhead.
const maxDataSize = 950000

// GetBuild fetches a buildbot build from the storage.
// Does not check access.
func GetBuild(c context.Context, master, builder string, number int) (*buildbot.Build, error) {
	entity := &buildEntity{
		Master:      master,
		Buildername: builder,
		Number:      number,
	}

	err := datastore.Get(c, entity)
	if err == datastore.ErrNoSuchEntity {
		err = errors.New("build not found", common.CodeNotFound)
	}

	return (*buildbot.Build)(entity), err
}

// ErrImportRejected is returned when an entity cannot be mutated
// anymore.
var ErrImportRejected = errors.New("import rejected")

// SaveBuild persists the build in the storage.
func SaveBuild(c context.Context, b *buildbot.Build) (replaced bool, err error) {
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		existing := &buildEntity{
			Master:      b.Master,
			Buildername: b.Buildername,
			Number:      b.Number,
		}
		switch err := datastore.Get(c, existing); {
		case err == datastore.ErrNoSuchEntity:
		case err != nil:
			return err
		case existing.Finished && b.Finished:
			return nil // idempotency
		case existing.Finished:
			return ErrImportRejected
		default:
			replaced = true
		}

		return datastore.Put(c, (*buildEntity)(b))
	}, nil)
	return
}

// BuildKey returns a key that identifies the build.
// An entity with that key does not necessarily exist.
func BuildKey(c context.Context, b *buildbot.Build) *datastore.Key {
	return datastore.KeyForObj(c, (*buildEntity)(b))
}

// buildEntity is a datstore entity that stores buildbot.Build in
// compressed JSON format.
// The properties is exclusively defined in Save/Load methods.
type buildEntity buildbot.Build

const buildKind = "buildbotBuild"

var _ datastore.PropertyLoadSaver = (*buildEntity)(nil)
var _ datastore.MetaGetterSetter = (*buildEntity)(nil)

// getID is a helper function that returns b's datastore key.
func (b *buildEntity) getID() string {
	s := []string{b.Master, b.Buildername, strconv.Itoa(b.Number)}
	id, err := json.Marshal(s)
	if err != nil {
		panic(err) // This can't fail.
	}
	return string(id)
}

// setID is the inverse of getID().
func (b *buildEntity) setID(id string) error {
	s := []string{}
	err := json.Unmarshal([]byte(id), &s)
	if err != nil {
		return err
	}
	if len(s) != 3 {
		return fmt.Errorf("%q does not have 3 items", id)
	}
	b.Master = s[0]
	b.Buildername = s[1]
	b.Number, err = strconv.Atoi(s[2])
	return err // or nil.
}

func (b *buildEntity) GetMeta(key string) (interface{}, bool) {
	switch key {
	case "id":
		return b.getID(), true
	case "kind":
		return buildKind, true
	default:
		return nil, false
	}
}

func (b *buildEntity) GetAllMeta() datastore.PropertyMap {
	return datastore.PropertyMap{
		"id":   datastore.MkPropertyNI(b.getID()),
		"kind": datastore.MkPropertyNI(buildKind),
	}
}

func (b *buildEntity) SetMeta(key string, val interface{}) bool {
	switch key {
	case "id":
		err := b.setID(val.(string))
		if err != nil {
			panic(err)
		}
		return true

	default:
		return false
	}
}

// Save converts b to a property map.
// The encoded build goes into "data" property.
// In addition, Save returns "master", "builder", "number" and "finished"
// properties for queries.
func (b *buildEntity) Save(withMeta bool) (datastore.PropertyMap, error) {
	var ps datastore.PropertyMap
	if withMeta {
		ps = b.GetAllMeta()
	} else {
		ps = datastore.PropertyMap{}
	}

	data, err := encode(b)
	if err != nil {
		return nil, err
	}
	if len(data) > maxDataSize {
		return nil, ErrTooBig
	}
	ps["data"] = datastore.MkPropertyNI(data)
	ps["master"] = datastore.MkProperty(b.Master)
	ps["builder"] = datastore.MkProperty(b.Buildername)
	ps["number"] = datastore.MkProperty(b.Number)
	ps["finished"] = datastore.MkProperty(b.Finished)
	return ps, nil
}

// Load loads b from the datastore property map.
// Also promotes LogDog links.
func (b *buildEntity) Load(pm datastore.PropertyMap) error {
	if p, ok := pm["id"]; ok {
		b.SetMeta("id", p.Slice()[0].Value())
	}

	if p, ok := pm["data"]; ok {
		data, err := p.Slice()[0].Project(datastore.PTBytes)
		if err != nil {
			return err
		}
		build := (*buildbot.Build)(b)
		if err := decode(build, data.([]byte)); err != nil {
			return err
		}
		promoteLogdogAliases(build)
	}

	return nil
}

// promoteLogdogAliases promotes LogDog links to first-class links.
func promoteLogdogAliases(b *buildbot.Build) {
	// If this is a LogDog-only build, we want to promote the LogDog links.
	if loc, ok := b.PropertyValue("log_location").(string); ok && strings.HasPrefix(loc, "logdog://") {
		linkMap := map[string]string{}
		for i := range b.Steps {
			promoteLogDogLinks(&b.Steps[i], i == 0, linkMap)
		}

		// Update "b.Logs". This field is part of BuildBot, and is the amalgamation
		// of all logs in the build's steps. Since each log is out of context of its
		// original step, we can't apply the promotion logic; instead, we will use
		// the link map to map any old URLs that were matched in "promoteLogDogLinks"
		// to their new URLs.
		for i := range b.Logs {
			l := &b.Logs[i]
			if newURL, ok := linkMap[l.URL]; ok {
				l.URL = newURL
			}
		}
	}
}

// promoteLogDogLinks updates the links in a BuildBot step to
// promote LogDog links.
//
// A build's links come in one of three forms:
//	- Log Links, which link directly to BuildBot build logs.
//	- URL Links, which are named links to arbitrary URLs.
//	- Aliases, which attach to the label in one of the other types of links and
//	  augment it with additional named links.
//
// LogDog uses aliases exclusively to attach LogDog logs to other links. When
// the build is LogDog-only, though, the original links are actually junk. What
// we want to do is remove the original junk links and replace them with their
// alias counterparts, so that the "natural" BuildBot links are actually LogDog
// links.
//
// As URLs are re-mapped, the supplied "linkMap" will be updated to map the old
// URLs to the new ones.
func promoteLogDogLinks(s *buildbot.Step, isInitialStep bool, linkMap map[string]string) {
	remainingAliases := stringset.New(len(s.Aliases))
	for a := range s.Aliases {
		remainingAliases.Add(a)
	}

	maybePromoteAliases := func(sl buildbot.Log, isLog bool) []buildbot.Log {
		// As a special case, if this is the first step ("steps" in BuildBot), we
		// will refrain from promoting aliases for "stdio", since "stdio" represents
		// the raw BuildBot logs.
		if isLog && isInitialStep && sl.Name == "stdio" {
			// No aliases, don't modify this log.
			return []buildbot.Log{sl}
		}

		// If there are no aliases, we should obviously not promote them. This will
		// be the case for pre-LogDog steps such as build setup.
		aliases := s.Aliases[sl.Name]
		if len(aliases) == 0 {
			return []buildbot.Log{sl}
		}

		// We have chosen to promote the aliases. Therefore, we will not include
		// them as aliases in the modified step.
		remainingAliases.Del(sl.Name)

		result := make([]buildbot.Log, len(aliases))
		for i, alias := range aliases {
			log := buildbot.Log{alias.Text, alias.URL}

			// Any link named "logdog" (Annotee cosmetic implementation detail) will
			// inherit the name of the original log.
			if isLog && log.Name == "logdog" {
				log.Name = sl.Name
			}

			result[i] = log
		}

		// If we performed mapping, add the OLD -> NEW URL mapping to linkMap.
		//
		// Since multiple aliases can apply to a single log, and we have to pick
		// one, here, we'll arbitrarily pick the last one. This is maybe more
		// consistent than the first one because linkMap, itself, will end up
		// holding the last mapping for any given URL.
		if len(result) > 0 {
			linkMap[sl.URL] = result[len(result)-1].URL
		}

		return result
	}

	// Update step logs.
	newLogs := make([]buildbot.Log, 0, len(s.Logs))
	for _, l := range s.Logs {
		newLogs = append(newLogs, maybePromoteAliases(l, true)...)
	}
	s.Logs = newLogs

	// Update step URLs.
	newURLs := make(map[string]string, len(s.Urls))
	for label, link := range s.Urls {
		urlLinks := maybePromoteAliases(buildbot.Log{label, link}, false)
		if len(urlLinks) > 0 {
			// Use the last URL link, since our URL map can only tolerate one link.
			// The expected case here is that len(urlLinks) == 1, though, but it's
			// possible that multiple aliases can be included for a single URL, so
			// we need to handle that.
			newValue := urlLinks[len(urlLinks)-1]
			newURLs[newValue.Name] = newValue.URL
		} else {
			newURLs[label] = link
		}
	}
	s.Urls = newURLs

	// Preserve any aliases that haven't been promoted.
	var newAliases map[string][]*buildbot.LinkAlias
	if l := remainingAliases.Len(); l > 0 {
		newAliases = make(map[string][]*buildbot.LinkAlias, l)
		remainingAliases.Iter(func(v string) bool {
			newAliases[v] = s.Aliases[v]
			return true
		})
	}
	s.Aliases = newAliases
}
