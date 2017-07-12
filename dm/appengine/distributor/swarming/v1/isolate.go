// Copyright 2016 The LUCI Authors.
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

package swarming

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/service/info"
	isolateservice "github.com/luci/luci-go/common/api/isolate/isolateservice/v1"
	swarm "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/isolated"
	"github.com/luci/luci-go/common/isolatedclient"
	"github.com/luci/luci-go/common/sync/parallel"
	sv1 "github.com/luci/luci-go/dm/api/distributor/swarming/v1"
	dm "github.com/luci/luci-go/dm/api/service/v1"
	"golang.org/x/net/context"
)

const prevPath = ".dm/previous_execution.json"
const descPath = ".dm/quest_description.json"

func mkFile(data []byte) *isolated.File {
	mode := 0444
	size := int64(len(data))
	return &isolated.File{
		Digest: isolated.HashBytes(data),
		Mode:   &mode,
		Size:   &size,
	}
}

func mkMsgFile(pb proto.Message) ([]byte, *isolated.File) {
	buf := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, pb); err != nil {
		panic(err)
	}
	data := buf.Bytes()
	return data, mkFile(data)
}

func mkIsolated(c context.Context, params *sv1.Parameters, prevFile, descFile *isolated.File) ([]byte, *isolated.File) {
	cmdReplacer := strings.NewReplacer(
		"${DM.PREVIOUS.EXECUTION.STATE:PATH}", prevPath,
		"${DM.QUEST.DATA.DESC:PATH}", descPath,
		"${DM.HOST}", info.DefaultVersionHostname(c),
	)

	iso := isolated.New()
	iso.Command = make([]string, len(params.Job.Command))
	for i, tok := range params.Job.Command {
		iso.Command[i] = cmdReplacer.Replace(tok)
	}
	iso.Includes = make(isolated.HexDigests, len(params.Job.Inputs.Isolated))
	for i, input := range params.Job.Inputs.Isolated {
		iso.Includes[i] = isolated.HexDigest(input.Id)
	}

	iso.Files[prevPath] = *prevFile
	iso.Files[descPath] = *descFile

	isoData, err := json.Marshal(iso)
	if err != nil {
		panic(err)
	}
	isoFile := mkFile(isoData)

	return isoData, isoFile
}

type isoChunk struct {
	data  []byte
	isIso bool
	file  *isolated.File
}

func pushIsolate(c context.Context, isolateURL string, chunks []isoChunk) error {
	dgsts := make([]*isolateservice.HandlersEndpointsV1Digest, len(chunks))
	for i, chnk := range chunks {
		dgsts[i] = &isolateservice.HandlersEndpointsV1Digest{
			Digest: string(chnk.file.Digest), Size: *chnk.file.Size,
			IsIsolated: chnk.isIso}
	}

	anonC, authC := httpClients(c)

	isoClient := isolatedclient.New(
		anonC, authC, isolateURL, isolatedclient.DefaultNamespace, nil, nil)
	states, err := isoClient.Contains(c, dgsts)
	if err != nil {
		err = errors.Annotate(err, "checking containment for %d digests", len(dgsts)).Err()
		return err
	}
	return parallel.FanOutIn(func(ch chan<- func() error) {
		for i, st := range states {
			if st != nil {
				i, st := i, st
				ch <- func() error {
					return isoClient.Push(c, st, isolatedclient.NewBytesSource(chunks[i].data))
				}
			}
		}
	})
}

func prepIsolate(c context.Context, isolateURL string, desc *dm.Quest_Desc, prev *dm.JsonResult, params *sv1.Parameters) (*swarm.SwarmingRpcsFilesRef, error) {
	prevData := []byte("{}")
	if prev != nil {
		prevData = []byte(prev.Object)
	}
	prevFile := mkFile(prevData)
	descData, descFile := mkMsgFile(desc)
	isoData, isoFile := mkIsolated(c, params, prevFile, descFile)

	err := pushIsolate(c, isolateURL, []isoChunk{
		{data: prevData, file: prevFile},
		{data: descData, file: descFile},
		{data: isoData, file: isoFile, isIso: true},
	})
	if err != nil {
		err = errors.Annotate(err, "pushing new Isolated").Err()
		return nil, err
	}

	return &swarm.SwarmingRpcsFilesRef{
		Isolated:       string(isoFile.Digest),
		Isolatedserver: isolateURL,
		Namespace:      isolatedclient.DefaultNamespace,
	}, nil
}
