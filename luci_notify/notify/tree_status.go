// Copyright 2020 The LUCI Authors.
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

package notify

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"golang.org/x/exp/slices"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	"go.chromium.org/luci/luci_notify/config"
)

var botUsernames = []string{
	"luci-notify@appspot.gserviceaccount.com",
	"luci-notify-dev@appspot.gserviceaccount.com",
	"buildbot@chromium.org", // Legacy bot.
}

type treeStatus struct {
	username           string
	message            string
	status             config.TreeCloserStatus
	timestamp          time.Time
	closingBuilderName string
}

type treeStatusClient interface {
	getStatus(c context.Context, treeName string) (*treeStatus, error)
	postStatus(c context.Context, message string, treeName string, status config.TreeCloserStatus, closingBuilderName string) error
}

type httpTreeStatusClient struct {
	client tspb.TreeStatusClient
}

func NewHTTPTreeStatusClient(ctx context.Context, luciTreeStatusHost string) (*httpTreeStatusClient, error) {
	transport, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.Insecure = lhttp.IsLocalHost(luciTreeStatusHost)
	prpcClient := &prpc.Client{
		C:                     &http.Client{Transport: transport},
		Host:                  luciTreeStatusHost,
		Options:               rpcOpts,
		MaxConcurrentRequests: 100,
	}

	return &httpTreeStatusClient{
		client: tspb.NewTreeStatusPRPCClient(prpcClient),
	}, nil
}

func (ts *httpTreeStatusClient) getStatus(ctx context.Context, treeName string) (*treeStatus, error) {
	request := &tspb.GetStatusRequest{
		Name: fmt.Sprintf("trees/%s/status/latest", treeName),
	}
	response, err := ts.client.GetStatus(ctx, request)
	if err != nil {
		return nil, err
	}

	var status = config.Closed
	if response.GeneralState == tspb.GeneralState_OPEN {
		status = config.Open
	}

	t := response.CreateTime.AsTime()

	return &treeStatus{
		username:           response.CreateUser,
		message:            response.Message,
		status:             status,
		timestamp:          t,
		closingBuilderName: response.ClosingBuilderName,
	}, nil
}

func (ts *httpTreeStatusClient) postStatus(ctx context.Context, message string, treeName string, status config.TreeCloserStatus, closingBuilderName string) error {
	logging.Infof(ctx, "Updating status for %s: %q", treeName, message)

	generalState := tspb.GeneralState_OPEN
	if status == config.Closed {
		generalState = tspb.GeneralState_CLOSED
	}
	request := &tspb.CreateStatusRequest{
		Parent: fmt.Sprintf("trees/%s/status", treeName),
		Status: &tspb.Status{
			GeneralState:       generalState,
			Message:            message,
			ClosingBuilderName: closingBuilderName,
		},
	}
	_, err := ts.client.CreateStatus(ctx, request)
	return err
}

// UpdateTreeStatus is the HTTP handler triggered by cron when it's time to
// check tree closers and update tree status if necessary.
func UpdateTreeStatus(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	settings, err := config.FetchSettings(ctx)
	if err != nil {
		return errors.Annotate(err, "fetching settings").Err()
	}
	client, err := NewHTTPTreeStatusClient(ctx, settings.LuciTreeStatusHost)
	if err != nil {
		return errors.Annotate(err, "creating tree status client").Err()
	}

	return transient.Tag.Apply(updateTrees(ctx, client))
}

// updateTrees fetches all TreeClosers from datastore, uses this to determine if
// any trees should be opened or closed, and makes the necessary updates.
func updateTrees(c context.Context, ts treeStatusClient) error {
	// The goal here is, for every project, to atomically fetch the config
	// for that project along with all TreeClosers within it. So if the
	// project config and the set of TreeClosers are updated at the same
	// time, we should always see either both updates, or neither. Also, we
	// want to do it without XG transactions.
	//
	// First we fetch keys for all the projects. Second, for every project,
	// we fetch the full config and all TreeClosers in a transaction. Since
	// these two steps aren't within a transaction, it's possible that
	// changes have occurred in between. But all cases are dealt with:
	//
	// * Updates to project config or TreeClosers aren't a problem since we
	//   only fetch them in the second step anyway.
	// * Deletions of projects are fine, since if we don't find them in the
	//   second fetch we just ignore that project and carry on.
	// * New projects are ignored, and picked up the next time we run.
	q := datastore.NewQuery("Project").KeysOnly(true)
	var projects []*config.Project
	if err := datastore.GetAll(c, q, &projects); err != nil {
		return errors.Annotate(err, "failed to get project keys").Err()
	}

	// Guards access to both treeClosers and closingEnabledProjects.
	mu := sync.Mutex{}
	var treeClosers []*config.TreeCloser
	closingEnabledProjects := stringset.New(0)

	err := parallel.WorkPool(32, func(ch chan<- func() error) {
		for _, project := range projects {
			project := project
			ch <- func() error {
				return datastore.RunInTransaction(c, func(c context.Context) error {
					switch err := datastore.Get(c, project); {
					// The project was deleted since the previous time we fetched it just above.
					// In this case, just move on, since the project is no more.
					case err == datastore.ErrNoSuchEntity:
						logging.Infof(c, "Project %s removed between queries, ignoring it", project.Name)
						return nil
					case err != nil:
						return errors.Annotate(err, "failed to get project").Tag(transient.Tag).Err()
					}

					q := datastore.NewQuery("TreeCloser").Ancestor(datastore.KeyForObj(c, project))
					var treeClosersForProject []*config.TreeCloser
					if err := datastore.GetAll(c, q, &treeClosersForProject); err != nil {
						return errors.Annotate(err, "failed to get tree closers").Tag(transient.Tag).Err()
					}

					for _, tc := range treeClosersForProject {
						if !config.TreeNameRE.MatchString(tc.TreeName) {
							return fmt.Errorf("old tree closer found in project %q, %q; pausing tree status updates until data migrated", project.Name, tc.TreeName)
						}
					}

					mu.Lock()
					defer mu.Unlock()
					logging.Debugf(c, "Appending tree closers for project: %v", project)
					treeClosers = append(treeClosers, treeClosersForProject...)
					if project.TreeClosingEnabled {
						closingEnabledProjects.Add(project.Name)
					}

					return nil
				}, nil)
			}
		}
	})
	if err != nil {
		return err
	}

	logging.Debugf(c, "closingEnabledProjects: %v", closingEnabledProjects)
	return parallel.WorkPool(32, func(ch chan<- func() error) {
		for tree, treeClosers := range groupTreeClosersByTree(treeClosers) {
			tree, treeClosers := tree, treeClosers
			ch <- func() error {
				c := logging.SetField(c, "tree-status-tree", tree)
				return updateTree(c, ts, treeClosers, closingEnabledProjects, tree)
			}
		}
	})
}

func groupTreeClosersByTree(treeClosers []*config.TreeCloser) map[string][]*config.TreeCloser {
	byTree := map[string][]*config.TreeCloser{}
	for _, tc := range treeClosers {
		byTree[tc.TreeName] = append(byTree[tc.TreeName], tc)
	}
	return byTree
}

func tcProject(tc *config.TreeCloser) string {
	return tc.BuilderKey.Parent().StringID()
}

func updateTree(c context.Context, ts treeStatusClient, treeClosers []*config.TreeCloser, closingEnabledProjects stringset.Set, treeName string) error {
	treeStatus, err := ts.getStatus(c, treeName)
	if err != nil {
		return err
	}

	// The state machine we want to implement:
	//
	// State                | Transitions
	// ==================== | ========================
	// Manually Closed      | Always leave unchanged.
	// Manually Opened      | Transition to automatically closed if a (tree-closer)
	//                      | build which started after the manual re-opening fails.
	// Automatically Closed | Transition to automatically opened if all (tree-closer) builds pass.
	// Automatically Opened | Transition to automatically closed if a (tree-closer) build is failing.
	//
	// Note: Open and Closed above are an abstraction over the true tree state,
	// which can also be in 'throttled' or 'maintenance' state.
	// The special 'throttled' and 'maintenance' states are interpreted as 'closed'
	// by getStatus above and only ever set manually, so they are never modified.
	isLastUpdateManual := !slices.Contains(botUsernames, treeStatus.username)
	if treeStatus.status == config.Closed && isLastUpdateManual {
		// Don't do anything if the tree was manually closed.
		logging.Debugf(c, "Tree is closed and last update was from non-bot user %s; not doing anything", treeStatus.username)
		return nil
	}

	logging.Debugf(c, "Scanning treeClosers for any belonging to a project with tree closing enabled: %v", treeClosers)
	anyEnabled := false
	for _, tc := range treeClosers {
		if closingEnabledProjects.Has(tcProject(tc)) {
			logging.Debugf(c, "Found such a treeCloser: %v", tc)
			anyEnabled = true
			break
		}
	}
	logging.Debugf(c, "anyEnabled = %v", anyEnabled)

	// Whether any build is failing.
	//
	// A failing build is necessary and sufficient information to close an
	// automatically opened tree.
	// However, while it is a necessary condition to close a manually opened
	// tree, it is not sufficient. Sufficient is only if one of the failing builds
	// started since the last manual open, see `oldestClosed`.
	//
	// If no builds are failing, this is sufficient information to re-open an
	// automatically closed tree. (But not a manually closed tree, that is never
	// automatically re-opened.)
	anyFailingBuild := false

	// The oldest failing build. This is used to justify any tree closure.
	// If the last tree status update was a manual open, this is constrained to
	// the oldest failing build that the started after the manual open.
	var oldestClosed *config.TreeCloser
	for _, tc := range treeClosers {
		// If any TreeClosers are from projects with tree closing enabled,
		// ignore any TreeClosers *not* from such projects. In general we don't
		// expect different projects to close the same tree, so we're okay with
		// not seeing dry run logging for these TreeClosers in this rare case.
		if anyEnabled && !closingEnabledProjects.Has(tcProject(tc)) {
			continue
		}

		// For opening the tree, we need to make sure *all* builders are
		// passing, not just those that have had new builds. Otherwise we'll
		// open the tree after any new green build, even if the builder that
		// caused us to close it is still failing.
		if tc.Status == config.Closed {
			logging.Debugf(c, "Found failing builder with message: %s", tc.Message)
			anyFailingBuild = true

			justifiesTreeClosure := false
			if isLastUpdateManual {
				// Only pay attention to failing builds from after the last update to
				// the tree. Otherwise we'll close the tree even after people manually
				// open it.
				//
				// We use the build start time instead of the finish time to only include
				// builds which included all code changes that were present in the tree
				// when it was manually opened.
				if tc.BuildCreateTime.After(treeStatus.timestamp) {
					justifiesTreeClosure = true
				}
			} else {
				// Last state update was automatic. When the tree is under automatic
				// control, all failing builds can justify closure.
				justifiesTreeClosure = true
			}
			if justifiesTreeClosure {
				// Keep track of the oldest failing build (by finish time) that can
				// justify tree closure. We use the oldest for determinism and to
				// assist explainability.
				if oldestClosed == nil || tc.Timestamp.Before(oldestClosed.Timestamp) {
					logging.Debugf(c, "Updating oldest failing builder")
					oldestClosed = tc
				}
			}
		}
	}

	var newStatus config.TreeCloserStatus
	if !anyFailingBuild {
		// We can open the tree, as no builders are failing, including builders
		// that haven't run since the last update to the tree.
		logging.Debugf(c, "No failing builders; new status is Open")
		newStatus = config.Open
	} else {
		// There is a failing build.
		if oldestClosed != nil {
			// We can close the tree, as at least one builder is able to justify
			// the closure. (E.g. has started since the tree was manually opened.)
			logging.Debugf(c, "At least one failing builder; new status is Closed")
			newStatus = config.Closed
		} else {
			// Some builders are failing, but they were already failing before the
			// last update. Don't do anything, so as not to close the tree after a
			// sheriff has manually opened it.
			logging.Debugf(c, "At least one failing builder, but there's a more recent status update; not doing anything")
			return nil
		}
	}

	if treeStatus.status == newStatus {
		// Don't do anything if the current status is already correct.
		logging.Debugf(c, "Current status is already correct; not doing anything")
		return nil
	}

	var message string
	var closingBuilderName string
	if newStatus == config.Open {
		message = fmt.Sprintf("Tree is open (Automatic: %s)", randomMessage(c))
	} else {
		message = fmt.Sprintf("Tree is closed (Automatic: %s)", oldestClosed.Message)
		closingBuilderName = generateClosingBuilderName(c, oldestClosed)
	}

	if anyEnabled {
		return ts.postStatus(c, message, treeName, newStatus, closingBuilderName)
	}
	logging.Infof(c, "Would update status for %s to %q", treeName, message)
	return nil
}

func generateClosingBuilderName(c context.Context, treeCloser *config.TreeCloser) string {
	// bucketBuilder is of the form <bucket>/<builder>
	bucketBuilder := treeCloser.BuilderKey.StringID()
	bucketBuilderPattern := `([a-z0-9\-_.]{1,100})/([a-zA-Z0-9\-_.\(\) ]{1,128})`
	bucketBuilderRE := regexp.MustCompile(`^` + bucketBuilderPattern + `$`)
	if !bucketBuilderRE.MatchString(bucketBuilder) {
		logging.Warningf(c, "bucketBuilder %q is not valid format", bucketBuilder)
		return ""
	}

	m := bucketBuilderRE.FindStringSubmatch(bucketBuilder)

	// Some very old TreeCloser entities in datastore are not of the form
	// bucket/builder. They used buildergroup instead. We do not support
	// those tree closer (and they should not cause any tree to close).
	// For those, we just return empty string.
	if m == nil {
		logging.Warningf(c, "bucketBuilder %q is not valid format", bucketBuilder)
		return ""
	}
	project := tcProject(treeCloser)
	return fmt.Sprintf("projects/%s/buckets/%s/builders/%s", project, m[1], m[2])
}

// Want more messages? CLs welcome!
var messages = []string{
	"('o')",
	"(｡>﹏<｡)",
	"☃",
	"☀ Tree is open ☀",
	"٩◔̯◔۶",
	"☺",
	"(´・ω・`)",
	"(｀・ω・´)",
	"(΄◞ิ౪◟ิ‵ )",
	"(╹◡╹)",
	"♩‿♩",
	"(/･ω･)/",
	" ʅ(◔౪◔ ) ʃ",
	"ᕙ(`▿´)ᕗ",
	"ヽ(^o^)丿",
	"\\(･ω･)/",
	"＼(^o^)／",
	"ｷﾀ━━━━(ﾟ∀ﾟ)━━━━ｯ!!",
	"ヽ(^。^)ノ",
	"(ﾟдﾟ)",
	"ヽ(´ω`*人*´ω`)ノ",
	" ﾟ+｡:.ﾟヽ(*´∀`)ﾉﾟ.:｡+ﾟ",
	"(゜ー゜＊）ネッ！",
	" ♪d(´▽｀)b♪オールオッケィ♪",
	"(ﾉ≧∀≦)ﾉ・‥…",
	"☆（ゝω・）vｷｬﾋﾟ",
	"ლ(╹◡╹ლ)",
	"ƪ(•̃͡ε•̃͡)∫ʃ",
	"(•_•)",
	"( ་ ⍸ ་ )",
	"(☉౪ ⊙)",
	"˙ ͜ʟ˙",
	"( ఠൠఠ )",
	"☆.｡.:*･ﾟ☆.｡.:*･ﾟ☆祝☆ﾟ･*:.｡.☆ﾟ･*:.｡.☆",
	"༼ꉺɷꉺ༽",
	"◉_◉",
	"ϵ( ‘Θ’ )϶",
	"ヾ(⌐■_■)ノ♪",
	"(◡‿◡✿)",
	"★.:ﾟ+｡☆ (●´v｀○)bｫﾒﾃﾞﾄd(○´v｀●)☆.:ﾟ+｡★",
	"(☆.☆)",
	"ｵﾒﾃﾞﾄｰ♪c(*ﾟｰ^)ﾉ*･'ﾟ☆｡.:*:･'☆'･:*:.",
	"☆.。.:*・°☆.。.:*・°☆",
	"ʕ •ᴥ•ʔ",
	"☼.☼",
	"⊂(・(ェ)・)⊃",
	"(ﾉ≧∇≦)ﾉ ﾐ ┸━┸",
	"¯\\_(ツ)_/¯",
	"UwU",
	"Paç fat!",
	"Sretno",
	"Hodně štěstí!",
	"Held og lykke!",
	"Veel geluk!",
	"Edu!",
	"lykkyä tykö",
	"Viel Glück!",
	"Καλή τύχη!",
	"Sok szerencsét kivánok!",
	"Gangi þér vel!",
	"Go n-éirí an t-ádh leat!",
	"Buona fortuna!",
	"Laimīgs gadījums!",
	"Sėkmės!",
	"Vill Gléck!",
	"Со среќа!",
	"Powodzenia!",
	"Boa sorte!",
	"Noroc!",
	"Срећно",
	"Veľa šťastia!",
	"Lycka till!",
	"Bona sort!",
	"Zorte on!",
	"Góða eydnu",
	"¡Boa fortuna!",
	"Bona fortuna!",
	"Xewqat sbieħ",
	"Aigh vie!",
	"Pob lwc!",
	" موفق باشيد",
	"İyi şanslar!",
	"Bonŝancon!",
	"祝你好运！",
	"祝你好運！",
	"頑張って！",
	"សំណាងល្អ ",
	"행운을 빌어요",
	"शुभ कामना ",
	"โชคดี!",
	"Chúc may mắn!",
	"بالتوفيق!",
	"Sterkte!",
	"Ke o lakaletsa mohlohonolo",
	"Uve nemhanza yakanaka",
	"Kila la kheri!",
	"Amathamsanqa",
	"Ngikufisela iwela!",
	"Bonne chance!",
	"¡Buena suerte!",
	"Good luck!",
	"Semoga Beruntung!",
	"Selamat Maju Jaya!",
	"Ia manuia",
	"Suwertehin ka sana",
	"Հաջողությո'ւն",
	"Іске сәт",
	"Амжилт хүсье",
	"удачі!",
	"Da legst di nieda!",
	"Gell, da schaugst?",
	"Ois Guade",
	"शुभ कामना!",
	"நல் வாழ்த்துக்கள் ",
	"అంతా శుభం కలగాలి! ",
	":')",
	":'D",
	"`,;)",
	"Tree is open (^O^)",
	"Thượng lộ bình an",
	"Tree is open now (ง '̀͜ '́ )ง",
	"ヽ(^o^)ノ",
	"Ahoy all is good!",
	"All's right with the world!",
	"Aloha",
}

func randomMessage(c context.Context) string {
	message := messages[mathrand.Intn(c, len(messages))]
	if message[len(message)-1] == ')' {
		return message + " "
	}
	return message
}
