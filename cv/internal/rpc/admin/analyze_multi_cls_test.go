// Copyright 2023 The LUCI Authors.
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

package admin

import (
	"testing"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

func TestCLGraph(t *testing.T) {
	t.Parallel()

	t.Run("HasRootCL", func(t *testing.T) {
		t.Run("2->1->0", func(t *testing.T) {
			cls := generateRunCL(3)
			setHardDepOn(cls[2], cls[1])
			setHardDepOn(cls[1], cls[0])
			if !makeCLGraph(cls).hasRootCL() {
				t.Fail()
			}
		})

		t.Run("2->1->0->2", func(t *testing.T) {
			cls := generateRunCL(3)
			setHardDepOn(cls[2], cls[1])
			setHardDepOn(cls[1], cls[0])
			setSoftDepOn(cls[0], cls[2])
			if !makeCLGraph(cls).hasRootCL() {
				t.Fail()
			}
		})

		t.Run("2->1->0->2", func(t *testing.T) {
			cls := generateRunCL(3)
			setSoftDepOn(cls[2], cls[1])
			setSoftDepOn(cls[1], cls[0])
			setSoftDepOn(cls[0], cls[2])
			if !makeCLGraph(cls).hasRootCL() {
				t.Fail()
			}
		})

		t.Run("2->1->0;3->1;", func(t *testing.T) {
			cls := generateRunCL(4)
			setHardDepOn(cls[2], cls[1])
			setHardDepOn(cls[1], cls[0])
			setSoftDepOn(cls[3], cls[1])
			if makeCLGraph(cls).hasRootCL() {
				t.Fail()
			}
		})
		t.Run("2->1->0->2;3->1;", func(t *testing.T) {
			cls := generateRunCL(4)
			setHardDepOn(cls[2], cls[1])
			setHardDepOn(cls[1], cls[0])
			setSoftDepOn(cls[0], cls[2])
			setSoftDepOn(cls[3], cls[1])
			if !makeCLGraph(cls).hasRootCL() {
				t.Fail()
			}
		})
	})

	t.Run("DotGraph", func(t *testing.T) {
		t.Run("2->1->0", func(t *testing.T) {
			cls := generateRunCL(3)
			setHardDepOn(cls[2], cls[1])
			setHardDepOn(cls[1], cls[0])
			dotGraph := makeCLGraph(cls).computeDotGraph()
			expected := `digraph {
  "gerrit/example.com/0" [href="https://example.com/c/0", target="_parent"]
  "gerrit/example.com/1" [href="https://example.com/c/1", target="_parent"]
  "gerrit/example.com/2" [href="https://example.com/c/2", target="_parent"]

  "gerrit/example.com/1" -> "gerrit/example.com/0"
  "gerrit/example.com/2" -> "gerrit/example.com/1"
}`
			if dotGraph != expected {
				t.Fatalf("expecting:\n%s\ngot:\n%s", expected, dotGraph)
			}
		})

		t.Run("2->1->0->2", func(t *testing.T) {
			cls := generateRunCL(3)
			setSoftDepOn(cls[2], cls[1])
			setSoftDepOn(cls[1], cls[0])
			setSoftDepOn(cls[0], cls[2])
			dotGraph := makeCLGraph(cls).computeDotGraph()
			expected := `digraph {
  "gerrit/example.com/0" [href="https://example.com/c/0", target="_parent"]
  "gerrit/example.com/1" [href="https://example.com/c/1", target="_parent"]
  "gerrit/example.com/2" [href="https://example.com/c/2", target="_parent"]

  "gerrit/example.com/0" -> "gerrit/example.com/2"[style="dotted"]
  "gerrit/example.com/1" -> "gerrit/example.com/0"[style="dotted"]
  "gerrit/example.com/2" -> "gerrit/example.com/1"[style="dotted"]
}`
			if dotGraph != expected {
				t.Fatalf("expecting:\n%s\ngot:\n%s", expected, dotGraph)
			}
		})
	})
}

// generate CL 0...n-1
func generateRunCL(n int) []*run.RunCL {
	runCLs := make([]*run.RunCL, n)
	for i := range runCLs {
		runCLs[i] = &run.RunCL{
			ID:         common.CLID(i),
			ExternalID: changelist.MustGobID("example.com", int64(i)),
			Detail: &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: "example.com",
					},
				},
			},
		}
	}
	return runCLs
}

func setHardDepOn(from, to *run.RunCL) {
	_, change, err := to.ExternalID.ParseGobID()
	if err != nil {
		panic(err)
	}
	from.Detail.GetGerrit().GitDeps = append(from.Detail.GetGerrit().GitDeps, &changelist.GerritGitDep{
		Change:    change,
		Immediate: true,
	})
}

func setSoftDepOn(from, to *run.RunCL) {
	host, change, err := to.ExternalID.ParseGobID()
	if err != nil {
		panic(err)
	}
	from.Detail.GetGerrit().SoftDeps = append(from.Detail.GetGerrit().SoftDeps, &changelist.GerritSoftDep{
		Host:   host,
		Change: change,
	})
}
