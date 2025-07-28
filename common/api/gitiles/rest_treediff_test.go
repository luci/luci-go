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

package gitiles

import (
	"context"
	"net/http"
	"testing"

	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

const fakeLogTreeDiffData = ")]}'\n" + `{
  "log": [
    {
      "commit": "8983b2db201b133c57c9e3f57d02f0ec6a7f2871",
      "tree": "b1e6757180406cb6f9ce3039fa9b75c3bb729331",
      "parents": [
        "4914490a7dfd1e421f6a72c4cc4eb7e2d53f978a"
      ],
      "author": {
        "name": "recipe-roller",
        "email": "recipe-roller@chromium.org",
        "time": "Wed Sep 20 23:10:41 2017"
      },
      "committer": {
        "name": "Commit Bot",
        "email": "commit-bot@chromium.org",
        "time": "Wed Sep 20 23:20:26 2017"
      },
      "message": "Roll recipe dependencies (trivial).\n\nThis is an automated CL created by the recipe roller. This CL rolls recipe\nchanges from upstream projects (e.g. depot_tools) into downstream projects\n(e.g. tools/build).\n\n\nMore info is at https://goo.gl/zkKdpD. Use https://goo.gl/noib3a to file a bug.\nbuild:\n  https://crrev.com/d03ed4cbbbd2c33920723b2503cb66b6c4bf7e96 Add -v to all invocations of provision_devices.py (bpastene@chromium.org)\n\n\nTBR\u003diannucci@chromium.org\n\nRecipe-Tryjob-Bypass-Reason: Autoroller\nBugdroid-Send-Email: False\nChange-Id: Ie13de0b5fe9b83ecaed5bbf9e72cbc0593697752\nReviewed-on: https://chromium-review.googlesource.com/675964\nReviewed-by: Recipe Roller \u003crecipe-roller@chromium.org\u003e\nCommit-Queue: Recipe Roller \u003crecipe-roller@chromium.org\u003e\n",
      "tree_diff": [
        {
          "type": "modify",
          "old_id": "bbc4e3f6914072a163b714191c71492fd946b118",
          "old_mode": 33188,
          "old_path": "infra/config/recipes.cfg",
          "new_id": "f04e427a40c00dc7fc149f4c52954d41bcf3d9cb",
          "new_mode": 33188,
          "new_path": "infra/config/recipes.cfg"
        },
        {
          "type": "modify",
          "old_id": "8d63485b0dcb75645c17d3125525c025baa9cfa8",
          "old_mode": 33188,
          "old_path": "recipes/README.recipes.md",
          "new_id": "b3b2e83a33c2397ed46638112a2d9162a393234d",
          "new_mode": 33188,
          "new_path": "recipes/README.recipes.md"
        }
      ]
    },
    {
      "commit": "4914490a7dfd1e421f6a72c4cc4eb7e2d53f978a",
      "tree": "238e32d50c69917375490aebdf6a1cbc65f496ed",
      "parents": [
        "dbed67c599faf07473865727beed7cc65e5f3147"
      ],
      "author": {
        "name": "Brandon Wylie",
        "email": "wylieb@chromium.org",
        "time": "Wed Sep 20 22:50:08 2017"
      },
      "committer": {
        "name": "Commit Bot",
        "email": "commit-bot@chromium.org",
        "time": "Wed Sep 20 23:16:16 2017"
      },
      "message": "[Findit] Flake Analyzer - Rework timeouts for dtpr pipeline.\n\nCurrently the dtpr has a target iterations that it wants to get\nrun, and uses that as a basis of calculation. The strategy is\nflipped where it has a certain time period it wants to fill\nand calculates the number of tests that it needs to fill it.\nAlong with this, now that flake swarming tasks are being deleted\nat the start of every new task, I added elpased second to the\nflake analysis model to  fill in the approximation of time.\n\nBug:764961\nChange-Id: Ib3b033f48c2b3f7ac72d1b5973aa2bd9876d9963\nReviewed-on: https://chromium-review.googlesource.com/666320\nCommit-Queue: Brandon Wylie \u003cwylieb@chromium.org\u003e\nReviewed-by: Jeffrey Li \u003clijeffrey@chromium.org\u003e\nReviewed-by: Shuotao Gao \u003cstgao@chromium.org\u003e\n",
      "tree_diff": [
        {
          "type": "modify",
          "old_id": "5e99bfe849b1272c4f93998c1a4e474d50f71d07",
          "old_mode": 33188,
          "old_path": "appengine/findit/model/flake/master_flake_analysis.py",
          "new_id": "19db9511283f35ab878071b68c212cbf10ff0d08",
          "new_mode": 33188,
          "new_path": "appengine/findit/model/flake/master_flake_analysis.py"
        },
        {
          "type": "modify",
          "old_id": "40536a3f184ae7c0db26263e822c1da548041a9a",
          "old_mode": 33188,
          "old_path": "appengine/findit/waterfall/flake/determine_true_pass_rate_pipeline.py",
          "new_id": "f27168be0a4e38240b27df3e76bf73df08c8c137",
          "new_mode": 33188,
          "new_path": "appengine/findit/waterfall/flake/determine_true_pass_rate_pipeline.py"
        },
        {
          "type": "modify",
          "old_id": "b51512266349757e2531ed5d35a8fe5594559071",
          "old_mode": 33188,
          "old_path": "appengine/findit/waterfall/flake/flake_analysis_util.py",
          "new_id": "2241c947e48fb8853bdb509355c54cb4c56c6546",
          "new_mode": 33188,
          "new_path": "appengine/findit/waterfall/flake/flake_analysis_util.py"
        },
        {
          "type": "modify",
          "old_id": "3c66ca68c36239213f762862276171b8226ade17",
          "old_mode": 33188,
          "old_path": "appengine/findit/waterfall/flake/test/determine_true_pass_rate_pipeline_test.py",
          "new_id": "711a1259cd0db909af40c7e529940c06cd2ba265",
          "new_mode": 33188,
          "new_path": "appengine/findit/waterfall/flake/test/determine_true_pass_rate_pipeline_test.py"
        },
        {
          "type": "modify",
          "old_id": "5fe2ab9acdd1a5b8c4a31e62fa7c0d7ccdadbb97",
          "old_mode": 33188,
          "old_path": "appengine/findit/waterfall/flake/test/flake_analysis_util_test.py",
          "new_id": "bf886194e8f359cbb49bfa6dc17d6e4376dae375",
          "new_mode": 33188,
          "new_path": "appengine/findit/waterfall/flake/test/flake_analysis_util_test.py"
        },
        {
          "type": "modify",
          "old_id": "bad7a80f8315a20ccf4f02c97148f6e843e893de",
          "old_mode": 33188,
          "old_path": "appengine/findit/waterfall/flake/test/update_flake_analysis_data_points_pipeline_test.py",
          "new_id": "f217c9981c9f3ecd39a9831216ca133be676607a",
          "new_mode": 33188,
          "new_path": "appengine/findit/waterfall/flake/test/update_flake_analysis_data_points_pipeline_test.py"
        },
        {
          "type": "modify",
          "old_id": "f098f9575b22990c35928d6af4e59c5c0944e48d",
          "old_mode": 33188,
          "old_path": "appengine/findit/waterfall/flake/update_flake_analysis_data_points_pipeline.py",
          "new_id": "8a7a2acff2058240dd645096649dc28a48cb61e9",
          "new_mode": 33188,
          "new_path": "appengine/findit/waterfall/flake/update_flake_analysis_data_points_pipeline.py"
        }
      ]
    }
	]
}
`

func TestLogWithTreeDiff(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Log with TreeDiff", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fakeLogTreeDiffData))
		})
		defer srv.Close()

		res, err := c.Log(ctx, &gitiles.LogRequest{
			Project:            "repo",
			Committish:         "8de6836858c99e48f3c58164ab717bda728e95dd",
			ExcludeAncestorsOf: "master",
			PageSize:           10,
			TreeDiff:           true,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(res.Log), should.Equal(2))
		assert.Loosely(t, res.Log[1].TreeDiff[0], should.Match(&git.Commit_TreeDiff{
			Type:    git.Commit_TreeDiff_MODIFY,
			OldId:   "5e99bfe849b1272c4f93998c1a4e474d50f71d07",
			OldMode: 33188,
			OldPath: "appengine/findit/model/flake/master_flake_analysis.py",
			NewId:   "19db9511283f35ab878071b68c212cbf10ff0d08",
			NewMode: 33188,
			NewPath: "appengine/findit/model/flake/master_flake_analysis.py",
		}))
	})
}
