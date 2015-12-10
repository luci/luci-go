// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/client/logdog/annotee"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/types"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/common/transport"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/appengine/gaeauth/client"
)

func resolveServer(server string) string {
	// TODO(hinoka): configure this map in luci-config
	if server == "" || server == "default" || server == "dev" {
		return "chromium-swarm-dev.appspot.com"
	} else if server == "prod" {
		return "chromium-swarm.appspot.com"
	} else {
		return server
	}
}

// swarmingIDs that beging with "debug:" wil redirect to json found in
// /testdata/
func getSwarmingLog(server string, swarmingID string, c context.Context) ([]byte, error) {
	// Fetch the debug file instead.
	if strings.HasPrefix(swarmingID, "debug:") {
		filename := strings.Join(
			[]string{"testdata", swarmingID[6:]}, "/")
		b, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		return b, nil
	}

	swarmingURL := fmt.Sprintf(
		"https://%s/_ah/api/swarming/v1/task/%s/stdout",
		resolveServer(server), swarmingID)
	client := transport.GetClient(client.UseServiceAccountTransport(c,
		[]string{"https://www.googleapis.com/auth/userinfo.email"}, nil))
	resp, err := client.Get(swarmingURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Failed to fetch %s, status code %d", swarmingURL, resp.StatusCode)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Decode the JSON and extract the actual log.
	sm := map[string]*string{}
	if err := json.Unmarshal(body, &sm); err != nil {
		return nil, err
	}

	// Decode the data using annotee.
	if output, ok := sm["output"]; ok {
		return []byte(*output), nil
	}
	return nil, fmt.Errorf("Swarming response did not contain output\n%s", body)
}

// TODO(hinoka): This should go in a more generic file, when milo has more
// than one page.
func getNavi(swarmingID string, URL string) *resp.Navigation {
	navi := &resp.Navigation{}
	navi.PageTitle = &resp.Link{
		Label: swarmingID,
		URL:   URL,
	}
	navi.SiteTitle = &resp.Link{
		Label: "Milo",
		URL:   "/",
	}
	return navi
}

// Given a logdog/milo step, translate it to a BuildComponent struct.
func miloBuildStep(
	url string, anno *miloProto.Step, name string) *resp.BuildComponent {
	comp := &resp.BuildComponent{}
	asc := anno.GetStepComponent()
	comp.Label = asc.Name
	switch asc.Status {
	case miloProto.Status_RUNNING:
		comp.Status = resp.Running

	case miloProto.Status_SUCCESS:
		comp.Status = resp.Success

	case miloProto.Status_FAILURE:
		if anno.GetFailureDetails() != nil {
			switch anno.GetFailureDetails().Type {
			case miloProto.FailureDetails_INFRA:
				comp.Status = resp.InfraFailure

			case miloProto.FailureDetails_DM_DEPENDENCY_FAILED:
				comp.Status = resp.DependencyFailure

			default:
				comp.Status = resp.Failure
			}
		} else {
			comp.Status = resp.Failure
		}

	case miloProto.Status_EXCEPTION:
		comp.Status = resp.InfraFailure

		// Missing the case of waiting on unfinished dependency...
	default:
		comp.Status = resp.NotRun
	}
	// Sub link is for one link per log that isn't stdio.
	for _, link := range asc.GetOtherLinks() {
		lds := link.GetLogdogStream()
		if lds == nil {
			continue // DNE???
		}
		shortName := lds.Name[5 : len(lds.Name)-2]
		if strings.HasSuffix(lds.Name, "annotations") || strings.HasSuffix(lds.Name, "stdio") {
			// Skip the special ones.
			continue
		}
		newLink := &resp.Link{
			Label: shortName,
			URL:   strings.Join([]string{url, lds.Name}, "/"),
		}
		comp.SubLink = append(comp.SubLink, newLink)
	}

	// Main link is a link to the stdio.
	comp.MainLink = &resp.Link{
		Label: "stdio",
		URL:   strings.Join([]string{url, name, "stdio"}, "/"),
	}

	// This should always be a step.
	comp.Type = resp.Step

	// This should always be 0
	comp.LevelsDeep = 0

	// Timeswamapts
	comp.Started = asc.Started.Time().Format(time.RFC3339)

	// This should be the exact same thing.
	comp.Text = asc.Text

	return comp
}

// Takes a butler client and return a fully populated milo build.
func buildFromClient(c context.Context, swarmingID string, url string, s *memoryClient) (*resp.MiloBuild, error) {
	// Build the basic page response.
	build := &resp.MiloBuild{}
	build.Navi = getNavi(swarmingID, url)
	build.CurrentTime = clock.Now(c).String()

	// Now Fetch the main annotation of the build.
	mainAnno := &miloProto.Step{}
	proto.Unmarshal(s.stream["annotations"].dg, mainAnno)

	// Now fill in each of the step components.
	// TODO(hinoka): This is totes cachable.
	for _, name := range mainAnno.SubstepLogdogNameBase {
		anno := &miloProto.Step{}
		fullname := strings.Join([]string{name, "annotations"}, "/")
		proto.Unmarshal(s.stream[fullname].dg, anno)
		build.Components = append(build.Components, miloBuildStep(url, anno, name))
	}

	// Take care of properties
	propGroup := &resp.PropertyGroup{
		GroupName: "Main",
	}
	for _, prop := range mainAnno.GetStepComponent().Property {
		propGroup.Property = append(propGroup.Property, &resp.Property{
			Key:   prop.Name,
			Value: prop.Value,
		})
	}
	build.PropertyGroup = append(build.PropertyGroup, propGroup)

	// And we're done!
	return build, nil
}

// Takes in an annotated log and returns a fully populated memory client.
func clientFromAnnotatedLog(ctx context.Context, log []byte) (*memoryClient, error) {
	c := &memoryClient{}
	p := annotee.Processor{
		Context:                ctx,
		Client:                 c,
		MetadataUpdateInterval: time.Hour * 24, // Neverrrrrr send incr updates.
	}
	is := annotee.Stream{
		Reader:           bytes.NewBuffer(log),
		Name:             types.StreamName("stdio"),
		Annotate:         true,
		StripAnnotations: true,
	}
	// If this ever has more than one stream then memoryClient needs to become
	// goroutine safe
	if err := p.RunStreams([]*annotee.Stream{&is}); err != nil {
		return nil, err
	}
	return c, nil
}

func swarmingBuildImpl(c context.Context, URL string, server string, id string) (*resp.MiloBuild, error) {
	// Fetch the data from Swarming
	body, err := getSwarmingLog(server, id, c)
	if err != nil {
		return nil, err
	}

	// Decode the data using annotee.
	client, err := clientFromAnnotatedLog(c, body)
	if err != nil {
		return nil, err
	}

	return buildFromClient(c, id, URL, client)
}
