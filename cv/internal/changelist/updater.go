// Copyright 2021 The LUCI Authors.
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

package changelist

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
)

const (
	// BatchCLUpdateTaskClass is the Task Class ID of the BatchUpdateCLTask,
	// which is enqueued only during a transaction.
	BatchUpdateCLTaskClass = "batch-update-cl"
	// CLUpdateTaskClass is the Task Class ID of the UpdateCLTask.
	UpdateCLTaskClass = "update-cl"

	// blindRefreshInterval sets interval between blind refreshes of a CL.
	blindRefreshInterval = time.Minute
	BlindRefreshInterval = blindRefreshInterval

	// knownRefreshInterval sets interval between refreshes of a CL when
	// updatedHint is known.
	knownRefreshInterval = 15 * time.Minute

	// autoRefreshAfter makes CLs worthy of "blind" refresh.
	//
	// "blind" refresh means that CL is already stored in Datastore and is up to
	// the date to the best knowledge of CV.
	autoRefreshAfter = 2 * time.Hour
)

// TODO(crbug/1290468): This is an emergency HACK to recover CV from the outage.
// These CLs are in the same CL stack. Updating these CLs has caused OOM in LUCI
// CV. This HACK should be removed once long term solution is implemented for
// crbug/1290468.
var ignored_eids = stringset.NewFromSlice("gerrit/chromium-review.googlesource.com/3414409", "gerrit/chromium-review.googlesource.com/3414408", "gerrit/chromium-review.googlesource.com/3414407", "gerrit/chromium-review.googlesource.com/3414406", "gerrit/chromium-review.googlesource.com/3414405", "gerrit/chromium-review.googlesource.com/3414404", "gerrit/chromium-review.googlesource.com/3414403", "gerrit/chromium-review.googlesource.com/3414402", "gerrit/chromium-review.googlesource.com/3414401", "gerrit/chromium-review.googlesource.com/3414400", "gerrit/chromium-review.googlesource.com/3414399", "gerrit/chromium-review.googlesource.com/3414398", "gerrit/chromium-review.googlesource.com/3414397", "gerrit/chromium-review.googlesource.com/3414396", "gerrit/chromium-review.googlesource.com/3414395", "gerrit/chromium-review.googlesource.com/3414394", "gerrit/chromium-review.googlesource.com/3414393", "gerrit/chromium-review.googlesource.com/3414392", "gerrit/chromium-review.googlesource.com/3414391", "gerrit/chromium-review.googlesource.com/3414390", "gerrit/chromium-review.googlesource.com/3414389", "gerrit/chromium-review.googlesource.com/3414388", "gerrit/chromium-review.googlesource.com/3414387", "gerrit/chromium-review.googlesource.com/3414386", "gerrit/chromium-review.googlesource.com/3414385", "gerrit/chromium-review.googlesource.com/3414384", "gerrit/chromium-review.googlesource.com/3414383", "gerrit/chromium-review.googlesource.com/3414382", "gerrit/chromium-review.googlesource.com/3414381", "gerrit/chromium-review.googlesource.com/3414380", "gerrit/chromium-review.googlesource.com/3414379", "gerrit/chromium-review.googlesource.com/3414378", "gerrit/chromium-review.googlesource.com/3414377", "gerrit/chromium-review.googlesource.com/3414376", "gerrit/chromium-review.googlesource.com/3414375", "gerrit/chromium-review.googlesource.com/3414374", "gerrit/chromium-review.googlesource.com/3414373", "gerrit/chromium-review.googlesource.com/3414372", "gerrit/chromium-review.googlesource.com/3414371", "gerrit/chromium-review.googlesource.com/3414210", "gerrit/chromium-review.googlesource.com/3414209", "gerrit/chromium-review.googlesource.com/3414208", "gerrit/chromium-review.googlesource.com/3414207", "gerrit/chromium-review.googlesource.com/3414206", "gerrit/chromium-review.googlesource.com/3414205", "gerrit/chromium-review.googlesource.com/3414204", "gerrit/chromium-review.googlesource.com/3414203", "gerrit/chromium-review.googlesource.com/3414202", "gerrit/chromium-review.googlesource.com/3414201", "gerrit/chromium-review.googlesource.com/3414200", "gerrit/chromium-review.googlesource.com/3414199", "gerrit/chromium-review.googlesource.com/3414198", "gerrit/chromium-review.googlesource.com/3414197", "gerrit/chromium-review.googlesource.com/3409919", "gerrit/chromium-review.googlesource.com/3409918", "gerrit/chromium-review.googlesource.com/3409917", "gerrit/chromium-review.googlesource.com/3409916", "gerrit/chromium-review.googlesource.com/3409915", "gerrit/chromium-review.googlesource.com/3409914", "gerrit/chromium-review.googlesource.com/3409913", "gerrit/chromium-review.googlesource.com/3409912", "gerrit/chromium-review.googlesource.com/3409911", "gerrit/chromium-review.googlesource.com/3409910", "gerrit/chromium-review.googlesource.com/3409909", "gerrit/chromium-review.googlesource.com/3409908", "gerrit/chromium-review.googlesource.com/3409907", "gerrit/chromium-review.googlesource.com/3409906", "gerrit/chromium-review.googlesource.com/3409905", "gerrit/chromium-review.googlesource.com/3409904", "gerrit/chromium-review.googlesource.com/3409903", "gerrit/chromium-review.googlesource.com/3409902", "gerrit/chromium-review.googlesource.com/3409901", "gerrit/chromium-review.googlesource.com/3409900", "gerrit/chromium-review.googlesource.com/3409899", "gerrit/chromium-review.googlesource.com/3409898", "gerrit/chromium-review.googlesource.com/3409897", "gerrit/chromium-review.googlesource.com/3409896", "gerrit/chromium-review.googlesource.com/3409895", "gerrit/chromium-review.googlesource.com/3409894", "gerrit/chromium-review.googlesource.com/3409893", "gerrit/chromium-review.googlesource.com/3409892", "gerrit/chromium-review.googlesource.com/3409891", "gerrit/chromium-review.googlesource.com/3409890", "gerrit/chromium-review.googlesource.com/3409889", "gerrit/chromium-review.googlesource.com/3409888", "gerrit/chromium-review.googlesource.com/3409887", "gerrit/chromium-review.googlesource.com/3409886", "gerrit/chromium-review.googlesource.com/3409885", "gerrit/chromium-review.googlesource.com/3409884", "gerrit/chromium-review.googlesource.com/3409883", "gerrit/chromium-review.googlesource.com/3409882", "gerrit/chromium-review.googlesource.com/3409881", "gerrit/chromium-review.googlesource.com/3409880", "gerrit/chromium-review.googlesource.com/3409879", "gerrit/chromium-review.googlesource.com/3409878", "gerrit/chromium-review.googlesource.com/3409877", "gerrit/chromium-review.googlesource.com/3409876", "gerrit/chromium-review.googlesource.com/3409875", "gerrit/chromium-review.googlesource.com/3409874", "gerrit/chromium-review.googlesource.com/3409873", "gerrit/chromium-review.googlesource.com/3409872", "gerrit/chromium-review.googlesource.com/3409871", "gerrit/chromium-review.googlesource.com/3409870", "gerrit/chromium-review.googlesource.com/3409869", "gerrit/chromium-review.googlesource.com/3409868", "gerrit/chromium-review.googlesource.com/3409867", "gerrit/chromium-review.googlesource.com/3409866", "gerrit/chromium-review.googlesource.com/3409865", "gerrit/chromium-review.googlesource.com/3409864", "gerrit/chromium-review.googlesource.com/3409863", "gerrit/chromium-review.googlesource.com/3409862", "gerrit/chromium-review.googlesource.com/3409861", "gerrit/chromium-review.googlesource.com/3409860", "gerrit/chromium-review.googlesource.com/3409859", "gerrit/chromium-review.googlesource.com/3409858", "gerrit/chromium-review.googlesource.com/3409857", "gerrit/chromium-review.googlesource.com/3409856", "gerrit/chromium-review.googlesource.com/3409855", "gerrit/chromium-review.googlesource.com/3409854", "gerrit/chromium-review.googlesource.com/3409853", "gerrit/chromium-review.googlesource.com/3409852", "gerrit/chromium-review.googlesource.com/3409851", "gerrit/chromium-review.googlesource.com/3409850", "gerrit/chromium-review.googlesource.com/3409849", "gerrit/chromium-review.googlesource.com/3409848", "gerrit/chromium-review.googlesource.com/3409847", "gerrit/chromium-review.googlesource.com/3409846", "gerrit/chromium-review.googlesource.com/3409845", "gerrit/chromium-review.googlesource.com/3409844", "gerrit/chromium-review.googlesource.com/3409843", "gerrit/chromium-review.googlesource.com/3409842", "gerrit/chromium-review.googlesource.com/3409841", "gerrit/chromium-review.googlesource.com/3409840", "gerrit/chromium-review.googlesource.com/3409839", "gerrit/chromium-review.googlesource.com/3409838", "gerrit/chromium-review.googlesource.com/3409837", "gerrit/chromium-review.googlesource.com/3409836", "gerrit/chromium-review.googlesource.com/3409835", "gerrit/chromium-review.googlesource.com/3409834", "gerrit/chromium-review.googlesource.com/3409833", "gerrit/chromium-review.googlesource.com/3409832", "gerrit/chromium-review.googlesource.com/3409831", "gerrit/chromium-review.googlesource.com/3409830", "gerrit/chromium-review.googlesource.com/3409829", "gerrit/chromium-review.googlesource.com/3409828", "gerrit/chromium-review.googlesource.com/3409827", "gerrit/chromium-review.googlesource.com/3409826", "gerrit/chromium-review.googlesource.com/3409825", "gerrit/chromium-review.googlesource.com/3409824", "gerrit/chromium-review.googlesource.com/3409823", "gerrit/chromium-review.googlesource.com/3409822", "gerrit/chromium-review.googlesource.com/3409821", "gerrit/chromium-review.googlesource.com/3409820", "gerrit/chromium-review.googlesource.com/3409819", "gerrit/chromium-review.googlesource.com/3409818", "gerrit/chromium-review.googlesource.com/3409817", "gerrit/chromium-review.googlesource.com/3409816", "gerrit/chromium-review.googlesource.com/3409815", "gerrit/chromium-review.googlesource.com/3409814", "gerrit/chromium-review.googlesource.com/3409813", "gerrit/chromium-review.googlesource.com/3409812", "gerrit/chromium-review.googlesource.com/3409811", "gerrit/chromium-review.googlesource.com/3409810", "gerrit/chromium-review.googlesource.com/3409809", "gerrit/chromium-review.googlesource.com/3409808", "gerrit/chromium-review.googlesource.com/3409807", "gerrit/chromium-review.googlesource.com/3409806", "gerrit/chromium-review.googlesource.com/3409805", "gerrit/chromium-review.googlesource.com/3409804", "gerrit/chromium-review.googlesource.com/3409803", "gerrit/chromium-review.googlesource.com/3409802", "gerrit/chromium-review.googlesource.com/3409801", "gerrit/chromium-review.googlesource.com/3409800", "gerrit/chromium-review.googlesource.com/3409799", "gerrit/chromium-review.googlesource.com/3409798", "gerrit/chromium-review.googlesource.com/3409797", "gerrit/chromium-review.googlesource.com/3409796", "gerrit/chromium-review.googlesource.com/3409795", "gerrit/chromium-review.googlesource.com/3409794", "gerrit/chromium-review.googlesource.com/3409793", "gerrit/chromium-review.googlesource.com/3409792", "gerrit/chromium-review.googlesource.com/3409791", "gerrit/chromium-review.googlesource.com/3409790", "gerrit/chromium-review.googlesource.com/3409789", "gerrit/chromium-review.googlesource.com/3409788", "gerrit/chromium-review.googlesource.com/3409787", "gerrit/chromium-review.googlesource.com/3409786", "gerrit/chromium-review.googlesource.com/3409785", "gerrit/chromium-review.googlesource.com/3409784", "gerrit/chromium-review.googlesource.com/3409783", "gerrit/chromium-review.googlesource.com/3409782", "gerrit/chromium-review.googlesource.com/3409781", "gerrit/chromium-review.googlesource.com/3409780", "gerrit/chromium-review.googlesource.com/3409779", "gerrit/chromium-review.googlesource.com/3409778", "gerrit/chromium-review.googlesource.com/3409777", "gerrit/chromium-review.googlesource.com/3409776", "gerrit/chromium-review.googlesource.com/3409775", "gerrit/chromium-review.googlesource.com/3409774", "gerrit/chromium-review.googlesource.com/3409773", "gerrit/chromium-review.googlesource.com/3409772", "gerrit/chromium-review.googlesource.com/3409771", "gerrit/chromium-review.googlesource.com/3409770", "gerrit/chromium-review.googlesource.com/3409769", "gerrit/chromium-review.googlesource.com/3409768", "gerrit/chromium-review.googlesource.com/3409767", "gerrit/chromium-review.googlesource.com/3409766", "gerrit/chromium-review.googlesource.com/3409765", "gerrit/chromium-review.googlesource.com/3409764", "gerrit/chromium-review.googlesource.com/3409763", "gerrit/chromium-review.googlesource.com/3409762", "gerrit/chromium-review.googlesource.com/3409761", "gerrit/chromium-review.googlesource.com/3409760", "gerrit/chromium-review.googlesource.com/3409759", "gerrit/chromium-review.googlesource.com/3409758", "gerrit/chromium-review.googlesource.com/3409757", "gerrit/chromium-review.googlesource.com/3409756", "gerrit/chromium-review.googlesource.com/3409755", "gerrit/chromium-review.googlesource.com/3409754", "gerrit/chromium-review.googlesource.com/3409753", "gerrit/chromium-review.googlesource.com/3409752", "gerrit/chromium-review.googlesource.com/3409751", "gerrit/chromium-review.googlesource.com/3409750", "gerrit/chromium-review.googlesource.com/3409749", "gerrit/chromium-review.googlesource.com/3409748", "gerrit/chromium-review.googlesource.com/3409747", "gerrit/chromium-review.googlesource.com/3409746", "gerrit/chromium-review.googlesource.com/3409745", "gerrit/chromium-review.googlesource.com/3409744", "gerrit/chromium-review.googlesource.com/3409743", "gerrit/chromium-review.googlesource.com/3409742", "gerrit/chromium-review.googlesource.com/3409741", "gerrit/chromium-review.googlesource.com/3409740", "gerrit/chromium-review.googlesource.com/3409739", "gerrit/chromium-review.googlesource.com/3409738", "gerrit/chromium-review.googlesource.com/3409737", "gerrit/chromium-review.googlesource.com/3409736", "gerrit/chromium-review.googlesource.com/3409735", "gerrit/chromium-review.googlesource.com/3409734", "gerrit/chromium-review.googlesource.com/3409733", "gerrit/chromium-review.googlesource.com/3409732", "gerrit/chromium-review.googlesource.com/3409731", "gerrit/chromium-review.googlesource.com/3409730", "gerrit/chromium-review.googlesource.com/3409729", "gerrit/chromium-review.googlesource.com/3409728", "gerrit/chromium-review.googlesource.com/3409727", "gerrit/chromium-review.googlesource.com/3409726", "gerrit/chromium-review.googlesource.com/3409725", "gerrit/chromium-review.googlesource.com/3409724", "gerrit/chromium-review.googlesource.com/3409723", "gerrit/chromium-review.googlesource.com/3409722", "gerrit/chromium-review.googlesource.com/3409721", "gerrit/chromium-review.googlesource.com/3409720", "gerrit/chromium-review.googlesource.com/3409719", "gerrit/chromium-review.googlesource.com/3409718", "gerrit/chromium-review.googlesource.com/3409717", "gerrit/chromium-review.googlesource.com/3409716", "gerrit/chromium-review.googlesource.com/3409715", "gerrit/chromium-review.googlesource.com/3409714", "gerrit/chromium-review.googlesource.com/3409713", "gerrit/chromium-review.googlesource.com/3409712", "gerrit/chromium-review.googlesource.com/3409711", "gerrit/chromium-review.googlesource.com/3409710", "gerrit/chromium-review.googlesource.com/3409709", "gerrit/chromium-review.googlesource.com/3409708", "gerrit/chromium-review.googlesource.com/3409707", "gerrit/chromium-review.googlesource.com/3409706", "gerrit/chromium-review.googlesource.com/3409705", "gerrit/chromium-review.googlesource.com/3409704", "gerrit/chromium-review.googlesource.com/3409703", "gerrit/chromium-review.googlesource.com/3409702", "gerrit/chromium-review.googlesource.com/3409701", "gerrit/chromium-review.googlesource.com/3409700", "gerrit/chromium-review.googlesource.com/3409699", "gerrit/chromium-review.googlesource.com/3409698", "gerrit/chromium-review.googlesource.com/3409697", "gerrit/chromium-review.googlesource.com/3409696", "gerrit/chromium-review.googlesource.com/3409695", "gerrit/chromium-review.googlesource.com/3409694", "gerrit/chromium-review.googlesource.com/3409693", "gerrit/chromium-review.googlesource.com/3409692", "gerrit/chromium-review.googlesource.com/3409691", "gerrit/chromium-review.googlesource.com/3409690", "gerrit/chromium-review.googlesource.com/3409689", "gerrit/chromium-review.googlesource.com/3409688", "gerrit/chromium-review.googlesource.com/3409687", "gerrit/chromium-review.googlesource.com/3409686", "gerrit/chromium-review.googlesource.com/3409685", "gerrit/chromium-review.googlesource.com/3409684", "gerrit/chromium-review.googlesource.com/3409683", "gerrit/chromium-review.googlesource.com/3409682", "gerrit/chromium-review.googlesource.com/3409681", "gerrit/chromium-review.googlesource.com/3409680", "gerrit/chromium-review.googlesource.com/3409679", "gerrit/chromium-review.googlesource.com/3409678", "gerrit/chromium-review.googlesource.com/3409677", "gerrit/chromium-review.googlesource.com/3409676", "gerrit/chromium-review.googlesource.com/3409675", "gerrit/chromium-review.googlesource.com/3409674", "gerrit/chromium-review.googlesource.com/3409673", "gerrit/chromium-review.googlesource.com/3409672", "gerrit/chromium-review.googlesource.com/3409671", "gerrit/chromium-review.googlesource.com/3409670", "gerrit/chromium-review.googlesource.com/3409669", "gerrit/chromium-review.googlesource.com/3409668", "gerrit/chromium-review.googlesource.com/3409667", "gerrit/chromium-review.googlesource.com/3409666", "gerrit/chromium-review.googlesource.com/3409665", "gerrit/chromium-review.googlesource.com/3409664", "gerrit/chromium-review.googlesource.com/3409663", "gerrit/chromium-review.googlesource.com/3409662", "gerrit/chromium-review.googlesource.com/3409661", "gerrit/chromium-review.googlesource.com/3409660", "gerrit/chromium-review.googlesource.com/3409659", "gerrit/chromium-review.googlesource.com/3409658", "gerrit/chromium-review.googlesource.com/3409657", "gerrit/chromium-review.googlesource.com/3409656", "gerrit/chromium-review.googlesource.com/3409655", "gerrit/chromium-review.googlesource.com/3409654", "gerrit/chromium-review.googlesource.com/3409653", "gerrit/chromium-review.googlesource.com/3409652", "gerrit/chromium-review.googlesource.com/3409651", "gerrit/chromium-review.googlesource.com/3409650", "gerrit/chromium-review.googlesource.com/3409649", "gerrit/chromium-review.googlesource.com/3409648", "gerrit/chromium-review.googlesource.com/3409647", "gerrit/chromium-review.googlesource.com/3409646", "gerrit/chromium-review.googlesource.com/3409645", "gerrit/chromium-review.googlesource.com/3409644", "gerrit/chromium-review.googlesource.com/3409643", "gerrit/chromium-review.googlesource.com/3409642", "gerrit/chromium-review.googlesource.com/3409641", "gerrit/chromium-review.googlesource.com/3409640", "gerrit/chromium-review.googlesource.com/3409639", "gerrit/chromium-review.googlesource.com/3409638", "gerrit/chromium-review.googlesource.com/3409637", "gerrit/chromium-review.googlesource.com/3409636", "gerrit/chromium-review.googlesource.com/3409635", "gerrit/chromium-review.googlesource.com/3409634", "gerrit/chromium-review.googlesource.com/3409633", "gerrit/chromium-review.googlesource.com/3409632", "gerrit/chromium-review.googlesource.com/3409631", "gerrit/chromium-review.googlesource.com/3409630", "gerrit/chromium-review.googlesource.com/3409629", "gerrit/chromium-review.googlesource.com/3409628", "gerrit/chromium-review.googlesource.com/3409627", "gerrit/chromium-review.googlesource.com/3409626", "gerrit/chromium-review.googlesource.com/3409625", "gerrit/chromium-review.googlesource.com/3409624", "gerrit/chromium-review.googlesource.com/3409623", "gerrit/chromium-review.googlesource.com/3409622", "gerrit/chromium-review.googlesource.com/3409621", "gerrit/chromium-review.googlesource.com/3409620", "gerrit/chromium-review.googlesource.com/3409619", "gerrit/chromium-review.googlesource.com/3409618", "gerrit/chromium-review.googlesource.com/3409617", "gerrit/chromium-review.googlesource.com/3409616", "gerrit/chromium-review.googlesource.com/3409615", "gerrit/chromium-review.googlesource.com/3409614", "gerrit/chromium-review.googlesource.com/3409613", "gerrit/chromium-review.googlesource.com/3409612", "gerrit/chromium-review.googlesource.com/3409611", "gerrit/chromium-review.googlesource.com/3409610", "gerrit/chromium-review.googlesource.com/3409609", "gerrit/chromium-review.googlesource.com/3409608", "gerrit/chromium-review.googlesource.com/3409607", "gerrit/chromium-review.googlesource.com/3409606", "gerrit/chromium-review.googlesource.com/3409605", "gerrit/chromium-review.googlesource.com/3409604", "gerrit/chromium-review.googlesource.com/3409603", "gerrit/chromium-review.googlesource.com/3409602", "gerrit/chromium-review.googlesource.com/3409601", "gerrit/chromium-review.googlesource.com/3409600", "gerrit/chromium-review.googlesource.com/3409599", "gerrit/chromium-review.googlesource.com/3409598", "gerrit/chromium-review.googlesource.com/3409597", "gerrit/chromium-review.googlesource.com/3409596", "gerrit/chromium-review.googlesource.com/3409595", "gerrit/chromium-review.googlesource.com/3409594", "gerrit/chromium-review.googlesource.com/3409593", "gerrit/chromium-review.googlesource.com/3409592", "gerrit/chromium-review.googlesource.com/3409591", "gerrit/chromium-review.googlesource.com/3409590", "gerrit/chromium-review.googlesource.com/3409589", "gerrit/chromium-review.googlesource.com/3409588", "gerrit/chromium-review.googlesource.com/3409587", "gerrit/chromium-review.googlesource.com/3409586", "gerrit/chromium-review.googlesource.com/3409585", "gerrit/chromium-review.googlesource.com/3409584", "gerrit/chromium-review.googlesource.com/3409583", "gerrit/chromium-review.googlesource.com/3409582", "gerrit/chromium-review.googlesource.com/3409581", "gerrit/chromium-review.googlesource.com/3409580", "gerrit/chromium-review.googlesource.com/3409579", "gerrit/chromium-review.googlesource.com/3409578", "gerrit/chromium-review.googlesource.com/3409577", "gerrit/chromium-review.googlesource.com/3409576", "gerrit/chromium-review.googlesource.com/3409575", "gerrit/chromium-review.googlesource.com/3409574", "gerrit/chromium-review.googlesource.com/3409573", "gerrit/chromium-review.googlesource.com/3409572", "gerrit/chromium-review.googlesource.com/3409571", "gerrit/chromium-review.googlesource.com/3409570", "gerrit/chromium-review.googlesource.com/3409569", "gerrit/chromium-review.googlesource.com/3409568", "gerrit/chromium-review.googlesource.com/3409567", "gerrit/chromium-review.googlesource.com/3409566", "gerrit/chromium-review.googlesource.com/3409565", "gerrit/chromium-review.googlesource.com/3409564", "gerrit/chromium-review.googlesource.com/3409563", "gerrit/chromium-review.googlesource.com/3409562", "gerrit/chromium-review.googlesource.com/3409561", "gerrit/chromium-review.googlesource.com/3409560", "gerrit/chromium-review.googlesource.com/3409559", "gerrit/chromium-review.googlesource.com/3409558", "gerrit/chromium-review.googlesource.com/3409557", "gerrit/chromium-review.googlesource.com/3409556", "gerrit/chromium-review.googlesource.com/3409555", "gerrit/chromium-review.googlesource.com/3409554", "gerrit/chromium-review.googlesource.com/3409553", "gerrit/chromium-review.googlesource.com/3409552", "gerrit/chromium-review.googlesource.com/3409551", "gerrit/chromium-review.googlesource.com/3409550", "gerrit/chromium-review.googlesource.com/3409549", "gerrit/chromium-review.googlesource.com/3409548", "gerrit/chromium-review.googlesource.com/3409547", "gerrit/chromium-review.googlesource.com/3409546", "gerrit/chromium-review.googlesource.com/3409545", "gerrit/chromium-review.googlesource.com/3409544", "gerrit/chromium-review.googlesource.com/3409543", "gerrit/chromium-review.googlesource.com/3409542", "gerrit/chromium-review.googlesource.com/3409541", "gerrit/chromium-review.googlesource.com/3409540", "gerrit/chromium-review.googlesource.com/3409539", "gerrit/chromium-review.googlesource.com/3409538", "gerrit/chromium-review.googlesource.com/3409537", "gerrit/chromium-review.googlesource.com/3409536", "gerrit/chromium-review.googlesource.com/3409535", "gerrit/chromium-review.googlesource.com/3409534", "gerrit/chromium-review.googlesource.com/3409533", "gerrit/chromium-review.googlesource.com/3409532", "gerrit/chromium-review.googlesource.com/3409531", "gerrit/chromium-review.googlesource.com/3409530", "gerrit/chromium-review.googlesource.com/3409529", "gerrit/chromium-review.googlesource.com/3409528", "gerrit/chromium-review.googlesource.com/3409527", "gerrit/chromium-review.googlesource.com/3409526", "gerrit/chromium-review.googlesource.com/3409525", "gerrit/chromium-review.googlesource.com/3409524", "gerrit/chromium-review.googlesource.com/3409523", "gerrit/chromium-review.googlesource.com/3409522", "gerrit/chromium-review.googlesource.com/3409521", "gerrit/chromium-review.googlesource.com/3409520", "gerrit/chromium-review.googlesource.com/3409519", "gerrit/chromium-review.googlesource.com/3409518", "gerrit/chromium-review.googlesource.com/3409517", "gerrit/chromium-review.googlesource.com/3409516", "gerrit/chromium-review.googlesource.com/3409515", "gerrit/chromium-review.googlesource.com/3409514", "gerrit/chromium-review.googlesource.com/3409513", "gerrit/chromium-review.googlesource.com/3409512", "gerrit/chromium-review.googlesource.com/3409511", "gerrit/chromium-review.googlesource.com/3409510", "gerrit/chromium-review.googlesource.com/3409509", "gerrit/chromium-review.googlesource.com/3409508", "gerrit/chromium-review.googlesource.com/3409507", "gerrit/chromium-review.googlesource.com/3409506", "gerrit/chromium-review.googlesource.com/3409505", "gerrit/chromium-review.googlesource.com/3409504", "gerrit/chromium-review.googlesource.com/3409503", "gerrit/chromium-review.googlesource.com/3409502", "gerrit/chromium-review.googlesource.com/3409501", "gerrit/chromium-review.googlesource.com/3409500", "gerrit/chromium-review.googlesource.com/3409499", "gerrit/chromium-review.googlesource.com/3409498", "gerrit/chromium-review.googlesource.com/3409497", "gerrit/chromium-review.googlesource.com/3409496", "gerrit/chromium-review.googlesource.com/3409495", "gerrit/chromium-review.googlesource.com/3409494", "gerrit/chromium-review.googlesource.com/3409493", "gerrit/chromium-review.googlesource.com/3409492", "gerrit/chromium-review.googlesource.com/3409491", "gerrit/chromium-review.googlesource.com/3409490", "gerrit/chromium-review.googlesource.com/3409489", "gerrit/chromium-review.googlesource.com/3409488", "gerrit/chromium-review.googlesource.com/3409487", "gerrit/chromium-review.googlesource.com/3409486", "gerrit/chromium-review.googlesource.com/3409485", "gerrit/chromium-review.googlesource.com/3409484", "gerrit/chromium-review.googlesource.com/3409483", "gerrit/chromium-review.googlesource.com/3409482", "gerrit/chromium-review.googlesource.com/3409481", "gerrit/chromium-review.googlesource.com/3409480", "gerrit/chromium-review.googlesource.com/3409479", "gerrit/chromium-review.googlesource.com/3409478", "gerrit/chromium-review.googlesource.com/3409477", "gerrit/chromium-review.googlesource.com/3409476", "gerrit/chromium-review.googlesource.com/3409475", "gerrit/chromium-review.googlesource.com/3409474", "gerrit/chromium-review.googlesource.com/3409473", "gerrit/chromium-review.googlesource.com/3409472", "gerrit/chromium-review.googlesource.com/3409471", "gerrit/chromium-review.googlesource.com/3409470", "gerrit/chromium-review.googlesource.com/3409469", "gerrit/chromium-review.googlesource.com/3409468", "gerrit/chromium-review.googlesource.com/3409467", "gerrit/chromium-review.googlesource.com/3409466", "gerrit/chromium-review.googlesource.com/3409465", "gerrit/chromium-review.googlesource.com/3409464", "gerrit/chromium-review.googlesource.com/3409463", "gerrit/chromium-review.googlesource.com/3409462", "gerrit/chromium-review.googlesource.com/3409461", "gerrit/chromium-review.googlesource.com/3409460", "gerrit/chromium-review.googlesource.com/3409459", "gerrit/chromium-review.googlesource.com/3409458", "gerrit/chromium-review.googlesource.com/3409457", "gerrit/chromium-review.googlesource.com/3409456", "gerrit/chromium-review.googlesource.com/3409455", "gerrit/chromium-review.googlesource.com/3409454", "gerrit/chromium-review.googlesource.com/3409453", "gerrit/chromium-review.googlesource.com/3409452", "gerrit/chromium-review.googlesource.com/3409451", "gerrit/chromium-review.googlesource.com/3409450", "gerrit/chromium-review.googlesource.com/3409449", "gerrit/chromium-review.googlesource.com/3409448", "gerrit/chromium-review.googlesource.com/3409447", "gerrit/chromium-review.googlesource.com/3409446", "gerrit/chromium-review.googlesource.com/3409445", "gerrit/chromium-review.googlesource.com/3409444", "gerrit/chromium-review.googlesource.com/3409443", "gerrit/chromium-review.googlesource.com/3409442", "gerrit/chromium-review.googlesource.com/3409441", "gerrit/chromium-review.googlesource.com/3409440", "gerrit/chromium-review.googlesource.com/3409439", "gerrit/chromium-review.googlesource.com/3409438", "gerrit/chromium-review.googlesource.com/3409437", "gerrit/chromium-review.googlesource.com/3409436", "gerrit/chromium-review.googlesource.com/3409435", "gerrit/chromium-review.googlesource.com/3409434", "gerrit/chromium-review.googlesource.com/3409433", "gerrit/chromium-review.googlesource.com/3409432", "gerrit/chromium-review.googlesource.com/3409431", "gerrit/chromium-review.googlesource.com/3409430", "gerrit/chromium-review.googlesource.com/3409429", "gerrit/chromium-review.googlesource.com/3409428", "gerrit/chromium-review.googlesource.com/3409427", "gerrit/chromium-review.googlesource.com/3409426", "gerrit/chromium-review.googlesource.com/3409425", "gerrit/chromium-review.googlesource.com/3409424", "gerrit/chromium-review.googlesource.com/3409423", "gerrit/chromium-review.googlesource.com/3409422", "gerrit/chromium-review.googlesource.com/3409421", "gerrit/chromium-review.googlesource.com/3409420", "gerrit/chromium-review.googlesource.com/3409419", "gerrit/chromium-review.googlesource.com/3409418", "gerrit/chromium-review.googlesource.com/3409417", "gerrit/chromium-review.googlesource.com/3409416", "gerrit/chromium-review.googlesource.com/3409415", "gerrit/chromium-review.googlesource.com/3409414", "gerrit/chromium-review.googlesource.com/3409413", "gerrit/chromium-review.googlesource.com/3409412", "gerrit/chromium-review.googlesource.com/3409411", "gerrit/chromium-review.googlesource.com/3409410", "gerrit/chromium-review.googlesource.com/3409409", "gerrit/chromium-review.googlesource.com/3409408", "gerrit/chromium-review.googlesource.com/3409407", "gerrit/chromium-review.googlesource.com/3409406", "gerrit/chromium-review.googlesource.com/3409405", "gerrit/chromium-review.googlesource.com/3409404", "gerrit/chromium-review.googlesource.com/3409403", "gerrit/chromium-review.googlesource.com/3409402", "gerrit/chromium-review.googlesource.com/3409401", "gerrit/chromium-review.googlesource.com/3409400", "gerrit/chromium-review.googlesource.com/3409399", "gerrit/chromium-review.googlesource.com/3409398", "gerrit/chromium-review.googlesource.com/3409397", "gerrit/chromium-review.googlesource.com/3409396", "gerrit/chromium-review.googlesource.com/3409395", "gerrit/chromium-review.googlesource.com/3409394", "gerrit/chromium-review.googlesource.com/3409393", "gerrit/chromium-review.googlesource.com/3409392", "gerrit/chromium-review.googlesource.com/3409391", "gerrit/chromium-review.googlesource.com/3409390", "gerrit/chromium-review.googlesource.com/3409389", "gerrit/chromium-review.googlesource.com/3409388", "gerrit/chromium-review.googlesource.com/3409387", "gerrit/chromium-review.googlesource.com/3409386", "gerrit/chromium-review.googlesource.com/3409385", "gerrit/chromium-review.googlesource.com/3409384", "gerrit/chromium-review.googlesource.com/3409383", "gerrit/chromium-review.googlesource.com/3409382", "gerrit/chromium-review.googlesource.com/3409381", "gerrit/chromium-review.googlesource.com/3409380", "gerrit/chromium-review.googlesource.com/3409379", "gerrit/chromium-review.googlesource.com/3409378", "gerrit/chromium-review.googlesource.com/3409377", "gerrit/chromium-review.googlesource.com/3409376", "gerrit/chromium-review.googlesource.com/3409375", "gerrit/chromium-review.googlesource.com/3409374", "gerrit/chromium-review.googlesource.com/3409373", "gerrit/chromium-review.googlesource.com/3409372", "gerrit/chromium-review.googlesource.com/3409371", "gerrit/chromium-review.googlesource.com/3409370", "gerrit/chromium-review.googlesource.com/3409369", "gerrit/chromium-review.googlesource.com/3409368", "gerrit/chromium-review.googlesource.com/3409367", "gerrit/chromium-review.googlesource.com/3409366", "gerrit/chromium-review.googlesource.com/3409365", "gerrit/chromium-review.googlesource.com/3409364", "gerrit/chromium-review.googlesource.com/3409363", "gerrit/chromium-review.googlesource.com/3409362", "gerrit/chromium-review.googlesource.com/3409361", "gerrit/chromium-review.googlesource.com/3409360", "gerrit/chromium-review.googlesource.com/3409359", "gerrit/chromium-review.googlesource.com/3409358", "gerrit/chromium-review.googlesource.com/3409357", "gerrit/chromium-review.googlesource.com/3409356", "gerrit/chromium-review.googlesource.com/3409355", "gerrit/chromium-review.googlesource.com/3409354", "gerrit/chromium-review.googlesource.com/3409353", "gerrit/chromium-review.googlesource.com/3409352", "gerrit/chromium-review.googlesource.com/3409351", "gerrit/chromium-review.googlesource.com/3409350", "gerrit/chromium-review.googlesource.com/3409349", "gerrit/chromium-review.googlesource.com/3409348", "gerrit/chromium-review.googlesource.com/3409347", "gerrit/chromium-review.googlesource.com/3409346", "gerrit/chromium-review.googlesource.com/3409345", "gerrit/chromium-review.googlesource.com/3409344", "gerrit/chromium-review.googlesource.com/3409343", "gerrit/chromium-review.googlesource.com/3409342", "gerrit/chromium-review.googlesource.com/3409341", "gerrit/chromium-review.googlesource.com/3409340", "gerrit/chromium-review.googlesource.com/3409339", "gerrit/chromium-review.googlesource.com/3409338", "gerrit/chromium-review.googlesource.com/3409337", "gerrit/chromium-review.googlesource.com/3409336", "gerrit/chromium-review.googlesource.com/3409335", "gerrit/chromium-review.googlesource.com/3409334", "gerrit/chromium-review.googlesource.com/3409333", "gerrit/chromium-review.googlesource.com/3409332", "gerrit/chromium-review.googlesource.com/3409331", "gerrit/chromium-review.googlesource.com/3409330", "gerrit/chromium-review.googlesource.com/3409329", "gerrit/chromium-review.googlesource.com/3409328", "gerrit/chromium-review.googlesource.com/3409327", "gerrit/chromium-review.googlesource.com/3409326", "gerrit/chromium-review.googlesource.com/3409325", "gerrit/chromium-review.googlesource.com/3409324", "gerrit/chromium-review.googlesource.com/3409323", "gerrit/chromium-review.googlesource.com/3409322", "gerrit/chromium-review.googlesource.com/3409321", "gerrit/chromium-review.googlesource.com/3409320", "gerrit/chromium-review.googlesource.com/3409319", "gerrit/chromium-review.googlesource.com/3409318", "gerrit/chromium-review.googlesource.com/3409317", "gerrit/chromium-review.googlesource.com/3409316", "gerrit/chromium-review.googlesource.com/3409315", "gerrit/chromium-review.googlesource.com/3409314", "gerrit/chromium-review.googlesource.com/3409313", "gerrit/chromium-review.googlesource.com/3409312", "gerrit/chromium-review.googlesource.com/3409311", "gerrit/chromium-review.googlesource.com/3409310", "gerrit/chromium-review.googlesource.com/3409309", "gerrit/chromium-review.googlesource.com/3409308", "gerrit/chromium-review.googlesource.com/3409307", "gerrit/chromium-review.googlesource.com/3409306", "gerrit/chromium-review.googlesource.com/3409305", "gerrit/chromium-review.googlesource.com/3409304", "gerrit/chromium-review.googlesource.com/3409303", "gerrit/chromium-review.googlesource.com/3409302", "gerrit/chromium-review.googlesource.com/3409301", "gerrit/chromium-review.googlesource.com/3409300", "gerrit/chromium-review.googlesource.com/3409299", "gerrit/chromium-review.googlesource.com/3409298", "gerrit/chromium-review.googlesource.com/3409297", "gerrit/chromium-review.googlesource.com/3409296", "gerrit/chromium-review.googlesource.com/3409295", "gerrit/chromium-review.googlesource.com/3409294", "gerrit/chromium-review.googlesource.com/3409293", "gerrit/chromium-review.googlesource.com/3409292", "gerrit/chromium-review.googlesource.com/3409291", "gerrit/chromium-review.googlesource.com/3409290", "gerrit/chromium-review.googlesource.com/3409289", "gerrit/chromium-review.googlesource.com/3408772", "gerrit/chromium-review.googlesource.com/3408771", "gerrit/chromium-review.googlesource.com/3408770", "gerrit/chromium-review.googlesource.com/3408769", "gerrit/chromium-review.googlesource.com/3408768", "gerrit/chromium-review.googlesource.com/3408767", "gerrit/chromium-review.googlesource.com/3408766", "gerrit/chromium-review.googlesource.com/3408765", "gerrit/chromium-review.googlesource.com/3408764", "gerrit/chromium-review.googlesource.com/3408763", "gerrit/chromium-review.googlesource.com/3408762", "gerrit/chromium-review.googlesource.com/3408761", "gerrit/chromium-review.googlesource.com/3408760", "gerrit/chromium-review.googlesource.com/3408759", "gerrit/chromium-review.googlesource.com/3408758", "gerrit/chromium-review.googlesource.com/3408757", "gerrit/chromium-review.googlesource.com/3408756", "gerrit/chromium-review.googlesource.com/3409288", "gerrit/chromium-review.googlesource.com/3409287", "gerrit/chromium-review.googlesource.com/3409286", "gerrit/chromium-review.googlesource.com/3409285", "gerrit/chromium-review.googlesource.com/3409284", "gerrit/chromium-review.googlesource.com/3409283", "gerrit/chromium-review.googlesource.com/3409282", "gerrit/chromium-review.googlesource.com/3409281", "gerrit/chromium-review.googlesource.com/3409280", "gerrit/chromium-review.googlesource.com/3409279", "gerrit/chromium-review.googlesource.com/3409278", "gerrit/chromium-review.googlesource.com/3409277", "gerrit/chromium-review.googlesource.com/3409276", "gerrit/chromium-review.googlesource.com/3409275", "gerrit/chromium-review.googlesource.com/3409274", "gerrit/chromium-review.googlesource.com/3409273", "gerrit/chromium-review.googlesource.com/3409272", "gerrit/chromium-review.googlesource.com/3409271", "gerrit/chromium-review.googlesource.com/3409270", "gerrit/chromium-review.googlesource.com/3409269", "gerrit/chromium-review.googlesource.com/3409268", "gerrit/chromium-review.googlesource.com/3409267", "gerrit/chromium-review.googlesource.com/3409266", "gerrit/chromium-review.googlesource.com/3409265", "gerrit/chromium-review.googlesource.com/3409264", "gerrit/chromium-review.googlesource.com/3409263", "gerrit/chromium-review.googlesource.com/3409262", "gerrit/chromium-review.googlesource.com/3409261", "gerrit/chromium-review.googlesource.com/3409260", "gerrit/chromium-review.googlesource.com/3409259", "gerrit/chromium-review.googlesource.com/3409258", "gerrit/chromium-review.googlesource.com/3409257", "gerrit/chromium-review.googlesource.com/3409256", "gerrit/chromium-review.googlesource.com/3409255", "gerrit/chromium-review.googlesource.com/3409254", "gerrit/chromium-review.googlesource.com/3409253", "gerrit/chromium-review.googlesource.com/3409252", "gerrit/chromium-review.googlesource.com/3409251", "gerrit/chromium-review.googlesource.com/3409250", "gerrit/chromium-review.googlesource.com/3409249", "gerrit/chromium-review.googlesource.com/3409248", "gerrit/chromium-review.googlesource.com/3409247", "gerrit/chromium-review.googlesource.com/3409246", "gerrit/chromium-review.googlesource.com/3409245", "gerrit/chromium-review.googlesource.com/3409244", "gerrit/chromium-review.googlesource.com/3409243", "gerrit/chromium-review.googlesource.com/3409242", "gerrit/chromium-review.googlesource.com/3409241", "gerrit/chromium-review.googlesource.com/3409240", "gerrit/chromium-review.googlesource.com/3409239", "gerrit/chromium-review.googlesource.com/3409238", "gerrit/chromium-review.googlesource.com/3409237", "gerrit/chromium-review.googlesource.com/3409236", "gerrit/chromium-review.googlesource.com/3409235", "gerrit/chromium-review.googlesource.com/3409234", "gerrit/chromium-review.googlesource.com/3409233", "gerrit/chromium-review.googlesource.com/3409232", "gerrit/chromium-review.googlesource.com/3409231", "gerrit/chromium-review.googlesource.com/3409230", "gerrit/chromium-review.googlesource.com/3409229", "gerrit/chromium-review.googlesource.com/3409228", "gerrit/chromium-review.googlesource.com/3409227", "gerrit/chromium-review.googlesource.com/3409226", "gerrit/chromium-review.googlesource.com/3409225", "gerrit/chromium-review.googlesource.com/3409224", "gerrit/chromium-review.googlesource.com/3409223", "gerrit/chromium-review.googlesource.com/3409222", "gerrit/chromium-review.googlesource.com/3409221", "gerrit/chromium-review.googlesource.com/3409220", "gerrit/chromium-review.googlesource.com/3409219", "gerrit/chromium-review.googlesource.com/3409218", "gerrit/chromium-review.googlesource.com/3409217", "gerrit/chromium-review.googlesource.com/3409216", "gerrit/chromium-review.googlesource.com/3409215", "gerrit/chromium-review.googlesource.com/3409214", "gerrit/chromium-review.googlesource.com/3409213", "gerrit/chromium-review.googlesource.com/3409212", "gerrit/chromium-review.googlesource.com/3409211", "gerrit/chromium-review.googlesource.com/3409210", "gerrit/chromium-review.googlesource.com/3409209", "gerrit/chromium-review.googlesource.com/3409207", "gerrit/chromium-review.googlesource.com/3409206", "gerrit/chromium-review.googlesource.com/3409205", "gerrit/chromium-review.googlesource.com/3409204", "gerrit/chromium-review.googlesource.com/3409203", "gerrit/chromium-review.googlesource.com/3409202", "gerrit/chromium-review.googlesource.com/3409201", "gerrit/chromium-review.googlesource.com/3409200", "gerrit/chromium-review.googlesource.com/3409199", "gerrit/chromium-review.googlesource.com/3409198", "gerrit/chromium-review.googlesource.com/3409197", "gerrit/chromium-review.googlesource.com/3409196", "gerrit/chromium-review.googlesource.com/3409195", "gerrit/chromium-review.googlesource.com/3409194", "gerrit/chromium-review.googlesource.com/3409193", "gerrit/chromium-review.googlesource.com/3409192", "gerrit/chromium-review.googlesource.com/3409191", "gerrit/chromium-review.googlesource.com/3409190", "gerrit/chromium-review.googlesource.com/3409189", "gerrit/chromium-review.googlesource.com/3409188", "gerrit/chromium-review.googlesource.com/3409187", "gerrit/chromium-review.googlesource.com/3409186", "gerrit/chromium-review.googlesource.com/3409185", "gerrit/chromium-review.googlesource.com/3409184", "gerrit/chromium-review.googlesource.com/3409183", "gerrit/chromium-review.googlesource.com/3409182", "gerrit/chromium-review.googlesource.com/3409181", "gerrit/chromium-review.googlesource.com/3409180", "gerrit/chromium-review.googlesource.com/3409179", "gerrit/chromium-review.googlesource.com/3409178", "gerrit/chromium-review.googlesource.com/3409177", "gerrit/chromium-review.googlesource.com/3409176", "gerrit/chromium-review.googlesource.com/3409175", "gerrit/chromium-review.googlesource.com/3409174", "gerrit/chromium-review.googlesource.com/3409173", "gerrit/chromium-review.googlesource.com/3409172", "gerrit/chromium-review.googlesource.com/3409171", "gerrit/chromium-review.googlesource.com/3409170", "gerrit/chromium-review.googlesource.com/3409169", "gerrit/chromium-review.googlesource.com/3409168", "gerrit/chromium-review.googlesource.com/3409167", "gerrit/chromium-review.googlesource.com/3409166", "gerrit/chromium-review.googlesource.com/3409165", "gerrit/chromium-review.googlesource.com/3409164", "gerrit/chromium-review.googlesource.com/3409163", "gerrit/chromium-review.googlesource.com/3409162", "gerrit/chromium-review.googlesource.com/3409161", "gerrit/chromium-review.googlesource.com/3409160", "gerrit/chromium-review.googlesource.com/3409159", "gerrit/chromium-review.googlesource.com/3409158", "gerrit/chromium-review.googlesource.com/3409157", "gerrit/chromium-review.googlesource.com/3409156", "gerrit/chromium-review.googlesource.com/3409155", "gerrit/chromium-review.googlesource.com/3409154", "gerrit/chromium-review.googlesource.com/3409153", "gerrit/chromium-review.googlesource.com/3409152", "gerrit/chromium-review.googlesource.com/3409151", "gerrit/chromium-review.googlesource.com/3409150", "gerrit/chromium-review.googlesource.com/3409149", "gerrit/chromium-review.googlesource.com/3409148", "gerrit/chromium-review.googlesource.com/3409147", "gerrit/chromium-review.googlesource.com/3409146", "gerrit/chromium-review.googlesource.com/3409145", "gerrit/chromium-review.googlesource.com/3409144", "gerrit/chromium-review.googlesource.com/3409143", "gerrit/chromium-review.googlesource.com/3409142", "gerrit/chromium-review.googlesource.com/3409141", "gerrit/chromium-review.googlesource.com/3409140", "gerrit/chromium-review.googlesource.com/3409139", "gerrit/chromium-review.googlesource.com/3409138", "gerrit/chromium-review.googlesource.com/3409137", "gerrit/chromium-review.googlesource.com/3409136", "gerrit/chromium-review.googlesource.com/3409135", "gerrit/chromium-review.googlesource.com/3409134", "gerrit/chromium-review.googlesource.com/3409133", "gerrit/chromium-review.googlesource.com/3409132", "gerrit/chromium-review.googlesource.com/3409131", "gerrit/chromium-review.googlesource.com/3409130", "gerrit/chromium-review.googlesource.com/3409129", "gerrit/chromium-review.googlesource.com/3409128", "gerrit/chromium-review.googlesource.com/3409127", "gerrit/chromium-review.googlesource.com/3409126", "gerrit/chromium-review.googlesource.com/3409125", "gerrit/chromium-review.googlesource.com/3409124", "gerrit/chromium-review.googlesource.com/3409123", "gerrit/chromium-review.googlesource.com/3409122", "gerrit/chromium-review.googlesource.com/3409121", "gerrit/chromium-review.googlesource.com/3409120", "gerrit/chromium-review.googlesource.com/3409119", "gerrit/chromium-review.googlesource.com/3409118", "gerrit/chromium-review.googlesource.com/3409117", "gerrit/chromium-review.googlesource.com/3409116", "gerrit/chromium-review.googlesource.com/3409115", "gerrit/chromium-review.googlesource.com/3409114", "gerrit/chromium-review.googlesource.com/3409113", "gerrit/chromium-review.googlesource.com/3409112", "gerrit/chromium-review.googlesource.com/3409111", "gerrit/chromium-review.googlesource.com/3409110", "gerrit/chromium-review.googlesource.com/3409109", "gerrit/chromium-review.googlesource.com/3409108", "gerrit/chromium-review.googlesource.com/3409107", "gerrit/chromium-review.googlesource.com/3409106", "gerrit/chromium-review.googlesource.com/3409105", "gerrit/chromium-review.googlesource.com/3409104", "gerrit/chromium-review.googlesource.com/3409103", "gerrit/chromium-review.googlesource.com/3409102", "gerrit/chromium-review.googlesource.com/3409101", "gerrit/chromium-review.googlesource.com/3409100", "gerrit/chromium-review.googlesource.com/3409099", "gerrit/chromium-review.googlesource.com/3409098", "gerrit/chromium-review.googlesource.com/3409097", "gerrit/chromium-review.googlesource.com/3409096", "gerrit/chromium-review.googlesource.com/3409095", "gerrit/chromium-review.googlesource.com/3409094", "gerrit/chromium-review.googlesource.com/3409093", "gerrit/chromium-review.googlesource.com/3409092", "gerrit/chromium-review.googlesource.com/3409091", "gerrit/chromium-review.googlesource.com/3409090", "gerrit/chromium-review.googlesource.com/3409089", "gerrit/chromium-review.googlesource.com/3409088", "gerrit/chromium-review.googlesource.com/3409087", "gerrit/chromium-review.googlesource.com/3409086", "gerrit/chromium-review.googlesource.com/3409085", "gerrit/chromium-review.googlesource.com/3409084", "gerrit/chromium-review.googlesource.com/3409083", "gerrit/chromium-review.googlesource.com/3409082", "gerrit/chromium-review.googlesource.com/3409081", "gerrit/chromium-review.googlesource.com/3409080", "gerrit/chromium-review.googlesource.com/3409079", "gerrit/chromium-review.googlesource.com/3409078", "gerrit/chromium-review.googlesource.com/3409077", "gerrit/chromium-review.googlesource.com/3409076", "gerrit/chromium-review.googlesource.com/3409075", "gerrit/chromium-review.googlesource.com/3409074", "gerrit/chromium-review.googlesource.com/3409073", "gerrit/chromium-review.googlesource.com/3409072", "gerrit/chromium-review.googlesource.com/3409071", "gerrit/chromium-review.googlesource.com/3409070", "gerrit/chromium-review.googlesource.com/3409069", "gerrit/chromium-review.googlesource.com/3409068", "gerrit/chromium-review.googlesource.com/3409067", "gerrit/chromium-review.googlesource.com/3409066", "gerrit/chromium-review.googlesource.com/3409065", "gerrit/chromium-review.googlesource.com/3409064", "gerrit/chromium-review.googlesource.com/3409063", "gerrit/chromium-review.googlesource.com/3409062", "gerrit/chromium-review.googlesource.com/3409061", "gerrit/chromium-review.googlesource.com/3409060", "gerrit/chromium-review.googlesource.com/3409059", "gerrit/chromium-review.googlesource.com/3409058", "gerrit/chromium-review.googlesource.com/3409057", "gerrit/chromium-review.googlesource.com/3409056", "gerrit/chromium-review.googlesource.com/3409055", "gerrit/chromium-review.googlesource.com/3409054", "gerrit/chromium-review.googlesource.com/3409053", "gerrit/chromium-review.googlesource.com/3409052", "gerrit/chromium-review.googlesource.com/3409051", "gerrit/chromium-review.googlesource.com/3409050", "gerrit/chromium-review.googlesource.com/3409049", "gerrit/chromium-review.googlesource.com/3409048", "gerrit/chromium-review.googlesource.com/3409047", "gerrit/chromium-review.googlesource.com/3409046", "gerrit/chromium-review.googlesource.com/3409045", "gerrit/chromium-review.googlesource.com/3409044", "gerrit/chromium-review.googlesource.com/3409043", "gerrit/chromium-review.googlesource.com/3409042", "gerrit/chromium-review.googlesource.com/3409041", "gerrit/chromium-review.googlesource.com/3409040", "gerrit/chromium-review.googlesource.com/3409039", "gerrit/chromium-review.googlesource.com/3409038", "gerrit/chromium-review.googlesource.com/3409037", "gerrit/chromium-review.googlesource.com/3409036", "gerrit/chromium-review.googlesource.com/3409035", "gerrit/chromium-review.googlesource.com/3409034", "gerrit/chromium-review.googlesource.com/3409033", "gerrit/chromium-review.googlesource.com/3409032", "gerrit/chromium-review.googlesource.com/3409031", "gerrit/chromium-review.googlesource.com/3409030", "gerrit/chromium-review.googlesource.com/3409029", "gerrit/chromium-review.googlesource.com/3409028", "gerrit/chromium-review.googlesource.com/3409027", "gerrit/chromium-review.googlesource.com/3409026", "gerrit/chromium-review.googlesource.com/3409025", "gerrit/chromium-review.googlesource.com/3409024", "gerrit/chromium-review.googlesource.com/3409023", "gerrit/chromium-review.googlesource.com/3409022", "gerrit/chromium-review.googlesource.com/3409021", "gerrit/chromium-review.googlesource.com/3409020", "gerrit/chromium-review.googlesource.com/3409019", "gerrit/chromium-review.googlesource.com/3409018", "gerrit/chromium-review.googlesource.com/3409017", "gerrit/chromium-review.googlesource.com/3409016", "gerrit/chromium-review.googlesource.com/3409015", "gerrit/chromium-review.googlesource.com/3409014", "gerrit/chromium-review.googlesource.com/3409013", "gerrit/chromium-review.googlesource.com/3409012", "gerrit/chromium-review.googlesource.com/3409011", "gerrit/chromium-review.googlesource.com/3409010", "gerrit/chromium-review.googlesource.com/3409009", "gerrit/chromium-review.googlesource.com/3409008", "gerrit/chromium-review.googlesource.com/3409007", "gerrit/chromium-review.googlesource.com/3409006", "gerrit/chromium-review.googlesource.com/3409005", "gerrit/chromium-review.googlesource.com/3409004", "gerrit/chromium-review.googlesource.com/3409003", "gerrit/chromium-review.googlesource.com/3409002", "gerrit/chromium-review.googlesource.com/3409001", "gerrit/chromium-review.googlesource.com/3409000", "gerrit/chromium-review.googlesource.com/3408999", "gerrit/chromium-review.googlesource.com/3408998", "gerrit/chromium-review.googlesource.com/3408997", "gerrit/chromium-review.googlesource.com/3408996", "gerrit/chromium-review.googlesource.com/3408995", "gerrit/chromium-review.googlesource.com/3408994", "gerrit/chromium-review.googlesource.com/3408993", "gerrit/chromium-review.googlesource.com/3408992", "gerrit/chromium-review.googlesource.com/3408991", "gerrit/chromium-review.googlesource.com/3408990", "gerrit/chromium-review.googlesource.com/3408989", "gerrit/chromium-review.googlesource.com/3408988", "gerrit/chromium-review.googlesource.com/3408987", "gerrit/chromium-review.googlesource.com/3408986", "gerrit/chromium-review.googlesource.com/3408985", "gerrit/chromium-review.googlesource.com/3408984", "gerrit/chromium-review.googlesource.com/3408983", "gerrit/chromium-review.googlesource.com/3408982", "gerrit/chromium-review.googlesource.com/3408981", "gerrit/chromium-review.googlesource.com/3408980", "gerrit/chromium-review.googlesource.com/3408979", "gerrit/chromium-review.googlesource.com/3408978", "gerrit/chromium-review.googlesource.com/3408977", "gerrit/chromium-review.googlesource.com/3408976", "gerrit/chromium-review.googlesource.com/3408975", "gerrit/chromium-review.googlesource.com/3408974", "gerrit/chromium-review.googlesource.com/3408973", "gerrit/chromium-review.googlesource.com/3408972", "gerrit/chromium-review.googlesource.com/3408971", "gerrit/chromium-review.googlesource.com/3408970", "gerrit/chromium-review.googlesource.com/3408969", "gerrit/chromium-review.googlesource.com/3408968", "gerrit/chromium-review.googlesource.com/3408967", "gerrit/chromium-review.googlesource.com/3408966", "gerrit/chromium-review.googlesource.com/3408965", "gerrit/chromium-review.googlesource.com/3408964", "gerrit/chromium-review.googlesource.com/3408963", "gerrit/chromium-review.googlesource.com/3408962", "gerrit/chromium-review.googlesource.com/3408961", "gerrit/chromium-review.googlesource.com/3408960", "gerrit/chromium-review.googlesource.com/3408959", "gerrit/chromium-review.googlesource.com/3408958", "gerrit/chromium-review.googlesource.com/3408957", "gerrit/chromium-review.googlesource.com/3408956", "gerrit/chromium-review.googlesource.com/3408955", "gerrit/chromium-review.googlesource.com/3408954", "gerrit/chromium-review.googlesource.com/3408953", "gerrit/chromium-review.googlesource.com/3408952", "gerrit/chromium-review.googlesource.com/3408951", "gerrit/chromium-review.googlesource.com/3408950", "gerrit/chromium-review.googlesource.com/3408949", "gerrit/chromium-review.googlesource.com/3408948", "gerrit/chromium-review.googlesource.com/3408947", "gerrit/chromium-review.googlesource.com/3408946", "gerrit/chromium-review.googlesource.com/3408945", "gerrit/chromium-review.googlesource.com/3408944", "gerrit/chromium-review.googlesource.com/3408943", "gerrit/chromium-review.googlesource.com/3408942", "gerrit/chromium-review.googlesource.com/3408941", "gerrit/chromium-review.googlesource.com/3408940", "gerrit/chromium-review.googlesource.com/3408939", "gerrit/chromium-review.googlesource.com/3408938", "gerrit/chromium-review.googlesource.com/3408937", "gerrit/chromium-review.googlesource.com/3408936", "gerrit/chromium-review.googlesource.com/3408935", "gerrit/chromium-review.googlesource.com/3408934", "gerrit/chromium-review.googlesource.com/3408933", "gerrit/chromium-review.googlesource.com/3408932", "gerrit/chromium-review.googlesource.com/3408931", "gerrit/chromium-review.googlesource.com/3408930", "gerrit/chromium-review.googlesource.com/3408929", "gerrit/chromium-review.googlesource.com/3408928", "gerrit/chromium-review.googlesource.com/3408927", "gerrit/chromium-review.googlesource.com/3408926", "gerrit/chromium-review.googlesource.com/3408925", "gerrit/chromium-review.googlesource.com/3408924", "gerrit/chromium-review.googlesource.com/3408923", "gerrit/chromium-review.googlesource.com/3408922", "gerrit/chromium-review.googlesource.com/3408921", "gerrit/chromium-review.googlesource.com/3408920", "gerrit/chromium-review.googlesource.com/3408919", "gerrit/chromium-review.googlesource.com/3408918", "gerrit/chromium-review.googlesource.com/3408917", "gerrit/chromium-review.googlesource.com/3408916", "gerrit/chromium-review.googlesource.com/3408915", "gerrit/chromium-review.googlesource.com/3408914", "gerrit/chromium-review.googlesource.com/3408913", "gerrit/chromium-review.googlesource.com/3408912", "gerrit/chromium-review.googlesource.com/3408911", "gerrit/chromium-review.googlesource.com/3408910", "gerrit/chromium-review.googlesource.com/3408909", "gerrit/chromium-review.googlesource.com/3408908", "gerrit/chromium-review.googlesource.com/3408907", "gerrit/chromium-review.googlesource.com/3408906", "gerrit/chromium-review.googlesource.com/3408905", "gerrit/chromium-review.googlesource.com/3408904", "gerrit/chromium-review.googlesource.com/3408903", "gerrit/chromium-review.googlesource.com/3408902", "gerrit/chromium-review.googlesource.com/3408901", "gerrit/chromium-review.googlesource.com/3408900", "gerrit/chromium-review.googlesource.com/3408899", "gerrit/chromium-review.googlesource.com/3408898", "gerrit/chromium-review.googlesource.com/3408897", "gerrit/chromium-review.googlesource.com/3408896", "gerrit/chromium-review.googlesource.com/3408895", "gerrit/chromium-review.googlesource.com/3408894", "gerrit/chromium-review.googlesource.com/3408893", "gerrit/chromium-review.googlesource.com/3408892", "gerrit/chromium-review.googlesource.com/3408891", "gerrit/chromium-review.googlesource.com/3408890", "gerrit/chromium-review.googlesource.com/3408889", "gerrit/chromium-review.googlesource.com/3408888", "gerrit/chromium-review.googlesource.com/3408887", "gerrit/chromium-review.googlesource.com/3408886", "gerrit/chromium-review.googlesource.com/3408885", "gerrit/chromium-review.googlesource.com/3408884", "gerrit/chromium-review.googlesource.com/3408883", "gerrit/chromium-review.googlesource.com/3408882", "gerrit/chromium-review.googlesource.com/3408881", "gerrit/chromium-review.googlesource.com/3408880", "gerrit/chromium-review.googlesource.com/3408879", "gerrit/chromium-review.googlesource.com/3408878", "gerrit/chromium-review.googlesource.com/3408877", "gerrit/chromium-review.googlesource.com/3408876", "gerrit/chromium-review.googlesource.com/3408875", "gerrit/chromium-review.googlesource.com/3408874", "gerrit/chromium-review.googlesource.com/3408873", "gerrit/chromium-review.googlesource.com/3408872", "gerrit/chromium-review.googlesource.com/3408871", "gerrit/chromium-review.googlesource.com/3408870", "gerrit/chromium-review.googlesource.com/3408869", "gerrit/chromium-review.googlesource.com/3408868", "gerrit/chromium-review.googlesource.com/3408867", "gerrit/chromium-review.googlesource.com/3408866", "gerrit/chromium-review.googlesource.com/3408865", "gerrit/chromium-review.googlesource.com/3408864", "gerrit/chromium-review.googlesource.com/3408863", "gerrit/chromium-review.googlesource.com/3408862", "gerrit/chromium-review.googlesource.com/3408861", "gerrit/chromium-review.googlesource.com/3408860", "gerrit/chromium-review.googlesource.com/3408859", "gerrit/chromium-review.googlesource.com/3408858", "gerrit/chromium-review.googlesource.com/3408857", "gerrit/chromium-review.googlesource.com/3408856", "gerrit/chromium-review.googlesource.com/3408855", "gerrit/chromium-review.googlesource.com/3408854", "gerrit/chromium-review.googlesource.com/3408853", "gerrit/chromium-review.googlesource.com/3408852", "gerrit/chromium-review.googlesource.com/3408851", "gerrit/chromium-review.googlesource.com/3408850", "gerrit/chromium-review.googlesource.com/3408849", "gerrit/chromium-review.googlesource.com/3408848", "gerrit/chromium-review.googlesource.com/3408847", "gerrit/chromium-review.googlesource.com/3408846", "gerrit/chromium-review.googlesource.com/3408845", "gerrit/chromium-review.googlesource.com/3408844", "gerrit/chromium-review.googlesource.com/3408843", "gerrit/chromium-review.googlesource.com/3408842", "gerrit/chromium-review.googlesource.com/3408841", "gerrit/chromium-review.googlesource.com/3408840", "gerrit/chromium-review.googlesource.com/3408839", "gerrit/chromium-review.googlesource.com/3408838", "gerrit/chromium-review.googlesource.com/3408837", "gerrit/chromium-review.googlesource.com/3408836", "gerrit/chromium-review.googlesource.com/3408835", "gerrit/chromium-review.googlesource.com/3408834", "gerrit/chromium-review.googlesource.com/3408833", "gerrit/chromium-review.googlesource.com/3408832", "gerrit/chromium-review.googlesource.com/3408831", "gerrit/chromium-review.googlesource.com/3408830", "gerrit/chromium-review.googlesource.com/3408829", "gerrit/chromium-review.googlesource.com/3408828", "gerrit/chromium-review.googlesource.com/3408827", "gerrit/chromium-review.googlesource.com/3408826", "gerrit/chromium-review.googlesource.com/3408825", "gerrit/chromium-review.googlesource.com/3408824", "gerrit/chromium-review.googlesource.com/3408823", "gerrit/chromium-review.googlesource.com/3408822", "gerrit/chromium-review.googlesource.com/3408821", "gerrit/chromium-review.googlesource.com/3408820", "gerrit/chromium-review.googlesource.com/3408819", "gerrit/chromium-review.googlesource.com/3408818", "gerrit/chromium-review.googlesource.com/3408817", "gerrit/chromium-review.googlesource.com/3408816", "gerrit/chromium-review.googlesource.com/3408815", "gerrit/chromium-review.googlesource.com/3408814", "gerrit/chromium-review.googlesource.com/3408813", "gerrit/chromium-review.googlesource.com/3408812", "gerrit/chromium-review.googlesource.com/3408811", "gerrit/chromium-review.googlesource.com/3408810", "gerrit/chromium-review.googlesource.com/3408809", "gerrit/chromium-review.googlesource.com/3408808", "gerrit/chromium-review.googlesource.com/3408807", "gerrit/chromium-review.googlesource.com/3408806", "gerrit/chromium-review.googlesource.com/3408805", "gerrit/chromium-review.googlesource.com/3408804", "gerrit/chromium-review.googlesource.com/3408803", "gerrit/chromium-review.googlesource.com/3408802", "gerrit/chromium-review.googlesource.com/3408801", "gerrit/chromium-review.googlesource.com/3408800", "gerrit/chromium-review.googlesource.com/3408799", "gerrit/chromium-review.googlesource.com/3408798", "gerrit/chromium-review.googlesource.com/3408797", "gerrit/chromium-review.googlesource.com/3408796", "gerrit/chromium-review.googlesource.com/3408795", "gerrit/chromium-review.googlesource.com/3408794", "gerrit/chromium-review.googlesource.com/3408793", "gerrit/chromium-review.googlesource.com/3408292", "gerrit/chromium-review.googlesource.com/3408291", "gerrit/chromium-review.googlesource.com/3408290")

// UpdaterBackend abstracts out fetching CL details from code review backend.
type UpdaterBackend interface {
	// Kind identifies the backend.
	//
	// It's also the first part of the CL's ExternalID, e.g. "gerrit".
	// Must not contain a slash.
	Kind() string

	// LookupApplicableConfig returns the latest ApplicableConfig for the previously
	// saved CL.
	//
	// See CL.ApplicableConfig field doc for more details. Roughly, it finds which
	// LUCI projects are configured to watch this CL.
	//
	// Updater calls LookupApplicableConfig() before Fetch() in order to avoid
	// the unnecessary Fetch() call entirely, e.g. if the CL is up to date or if
	// the CL is definitely not watched by a specific LUCI project.
	//
	// Returns non-nil ApplicableConfig normally.
	// Returns nil ApplicableConfig if the previously saved CL state isn't
	// sufficient to confidently determine the ApplicableConfig.
	LookupApplicableConfig(ctx context.Context, saved *CL) (*ApplicableConfig, error)

	// Fetch fetches the CL in the context of a given project.
	//
	// If a given cl.ID is 0, it means the CL entity doesn't exist in Datastore.
	// The cl.ExternalID is always set.
	//
	// UpdatedHint, if not zero time, is the backend-originating timestamp of the
	// most recent CL update time. It's sourced by CV by e.g. polling or PubSub
	// subscription. It is useful to detect and work around backend's eventual
	// consistency.
	Fetch(ctx context.Context, cl *CL, luciProject string, updatedHint time.Time) (UpdateFields, error)

	// TQErrorSpec allows customizing logging and error TQ-specific handling.
	//
	// For example, Gerrit backend may wish to retry out of quota errors without
	// logging detailed stacktrace.
	TQErrorSpec() common.TQIfy
}

// UpdateFields defines what parts of CL to update.
//
// At least one field must be specified.
type UpdateFields struct {
	// Snapshot overwrites existing CL snapshot if newer according to its
	// .ExternalUpdateTime.
	Snapshot *Snapshot

	// ApplicableConfig overwrites existing CL ApplicableConfig if semantically
	// different from existing one.
	ApplicableConfig *ApplicableConfig

	// AddDependentMeta adds or overwrites metadata per LUCI project in CL AsDepMeta.
	// Doesn't affect metadata stored for projects not referenced here.
	AddDependentMeta *Access

	// DelAccess deletes Access records for the given projects.
	DelAccess []string
}

// IsEmpty returns true if no updates are necessary.
func (u UpdateFields) IsEmpty() bool {
	return (u.Snapshot == nil &&
		u.ApplicableConfig == nil &&
		len(u.AddDependentMeta.GetByProject()) == 0 &&
		len(u.DelAccess) == 0)
}

func (u UpdateFields) shouldUpdateSnapshot(cl *CL) bool {
	switch {
	case u.Snapshot == nil:
		return false
	case cl.Snapshot == nil:
		return true
	case cl.Snapshot.GetOutdated() != nil:
		return true
	case u.Snapshot.GetExternalUpdateTime().AsTime().After(cl.Snapshot.GetExternalUpdateTime().AsTime()):
		return true
	case cl.Snapshot.GetLuciProject() != u.Snapshot.GetLuciProject():
		return true
	default:
		return false
	}
}

func (u UpdateFields) Apply(cl *CL) (changed bool) {
	if u.ApplicableConfig != nil && !cl.ApplicableConfig.SemanticallyEqual(u.ApplicableConfig) {
		cl.ApplicableConfig = u.ApplicableConfig
		changed = true
	}

	if u.shouldUpdateSnapshot(cl) {
		cl.Snapshot = u.Snapshot
		changed = true
	}

	switch {
	case u.AddDependentMeta == nil:
	case cl.Access == nil || cl.Access.GetByProject() == nil:
		cl.Access = u.AddDependentMeta
		changed = true
	default:
		e := cl.Access.GetByProject()
		for lProject, v := range u.AddDependentMeta.GetByProject() {
			if v.GetNoAccessTime() == nil {
				panic("NoAccessTime must be set")
			}
			old, exists := e[lProject]
			if !exists || old.GetUpdateTime().AsTime().Before(v.GetUpdateTime().AsTime()) {
				if old.GetNoAccessTime() != nil && old.GetNoAccessTime().AsTime().Before(v.GetNoAccessTime().AsTime()) {
					v.NoAccessTime = old.NoAccessTime
				}
				e[lProject] = v
				changed = true
			}
		}
	}

	if len(u.DelAccess) > 0 && len(cl.Access.GetByProject()) > 0 {
		for _, p := range u.DelAccess {
			if _, exists := cl.Access.GetByProject()[p]; exists {
				changed = true
				delete(cl.Access.ByProject, p)
				if len(cl.Access.GetByProject()) == 0 {
					cl.Access = nil
					break
				}
			}
		}
	}

	return
}

// Updater knows how to update CLs from relevant backend (e.g. Gerrit),
// notifying other CV parts as needed.
type Updater struct {
	tqd     *tq.Dispatcher
	mutator *Mutator

	rwmutex  sync.RWMutex // guards `backends`
	backends map[string]UpdaterBackend
}

// NewUpdater creates a new Updater.
//
// Starts without backends, but they ought to be added via RegisterBackend().
func NewUpdater(tqd *tq.Dispatcher, m *Mutator) *Updater {
	u := &Updater{
		tqd:      tqd,
		mutator:  m,
		backends: make(map[string]UpdaterBackend, 1),
	}
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           BatchUpdateCLTaskClass,
		Prototype:    &BatchUpdateCLTask{},
		Queue:        "update-cl",
		Quiet:        true,
		QuietOnError: true,
		Kind:         tq.Transactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*BatchUpdateCLTask)
			err := u.handleBatch(ctx, t)
			return common.TQifyError(ctx, err)
		},
	})
	tqd.RegisterTaskClass(tq.TaskClass{
		ID:           UpdateCLTaskClass,
		Prototype:    &UpdateCLTask{},
		Queue:        "update-cl",
		Quiet:        true,
		QuietOnError: true,
		Kind:         tq.FollowsContext,
		Handler: func(ctx context.Context, payload proto.Message) error {
			t := payload.(*UpdateCLTask)
			// NOTE: unlike other TQ handlers code in CV, the common.TQifyError is
			// done inside the handler to allow per-backend definition of which errors
			// are retriable.
			return u.handleCL(ctx, t)
		},
	})
	return u
}

// RegisterBackend registers a backend.
//
// Panics if backend for the same kind is already registered.
func (u *Updater) RegisterBackend(b UpdaterBackend) {
	kind := b.Kind()
	if strings.ContainsRune(kind, '/') {
		panic(fmt.Errorf("backend %T of kind %q must not contain '/'", b, kind))
	}
	u.rwmutex.Lock()
	defer u.rwmutex.Unlock()
	if _, exists := u.backends[kind]; exists {
		panic(fmt.Errorf("backend %q is already registered", kind))
	}
	u.backends[kind] = b
}

// ScheduleBatch schedules update of several CLs.
//
// If called in a transaction, enqueues exactly one TQ task transactionally.
// This allows to write 1 Datastore entity during a transaction instead of N
// entities if Schedule() was used for each CL.
//
// Otherwise, enqueues 1 TQ task per CL non-transactionally and in parallel.
func (u *Updater) ScheduleBatch(ctx context.Context, luciProject string, cls []*CL) error {
	tasks := make([]*UpdateCLTask, len(cls))
	for i, cl := range cls {
		tasks[i] = &UpdateCLTask{
			LuciProject: luciProject,
			ExternalId:  string(cl.ExternalID),
			Id:          int64(cl.ID),
		}
	}

	switch {
	case len(tasks) == 1:
		// Optimization for the most frequent use-case of single-CL Runs.
		return u.Schedule(ctx, tasks[0])
	case datastore.CurrentTransaction(ctx) == nil:
		return u.handleBatch(ctx, &BatchUpdateCLTask{Tasks: tasks})
	default:
		return u.tqd.AddTask(ctx, &tq.Task{
			Payload: &BatchUpdateCLTask{Tasks: tasks},
			Title:   fmt.Sprintf("batch-%s-%d", luciProject, len(tasks)),
		})
	}
}

// Schedule dispatches a TQ task. It should be used instead of the direct
// tq.AddTask to allow for consistent de-duplication.
func (u *Updater) Schedule(ctx context.Context, payload *UpdateCLTask) error {
	return u.ScheduleDelayed(ctx, payload, 0)
}

// ScheduleDelayed is the same as Schedule but with a delay.
func (u *Updater) ScheduleDelayed(ctx context.Context, payload *UpdateCLTask, delay time.Duration) error {
	task := &tq.Task{
		Payload: payload,
		Delay:   delay,
		Title:   makeTQTitleForHumans(payload),
	}
	if datastore.CurrentTransaction(ctx) == nil {
		task.DeduplicationKey = makeTaskDeduplicationKey(ctx, payload, delay)
	}
	return u.tqd.AddTask(ctx, task)
}

// ResolveAndScheduleDepsUpdate resolves deps, creating new CL entities as
// necessary, and schedules an update task for each dep which needs an update.
//
// It's meant to be used by the Updater backends.
//
// Returns a sorted slice of Deps by their CL ID, ready to be stored as
// CL.Snapshot.Deps.
func (u *Updater) ResolveAndScheduleDepsUpdate(ctx context.Context, luciProject string, deps map[ExternalID]DepKind) ([]*Dep, error) {
	// Optimize for the most frequent case whereby deps are already known to CV
	// and were updated recently enough that no task scheduling is even necessary.

	// Batch-resolve external IDs to CLIDs, and load all existing CLs.
	resolvingDeps, err := resolveDeps(ctx, luciProject, deps)
	if err != nil {
		return nil, err
	}
	// Identify indexes of deps which need to have an update task scheduled.
	ret := make([]*Dep, len(deps))
	var toSchedule []int // indexes
	for i, d := range resolvingDeps {
		if d.ready {
			ret[i] = d.resolvedDep
		} else {
			// Also covers the case of a dep not yet having a CL entity.
			toSchedule = append(toSchedule, i)
		}
	}
	if len(toSchedule) == 0 {
		// Quick path exit.
		return sortDeps(ret), nil
	}

	errs := parallel.WorkPool(min(10, len(toSchedule)), func(work chan<- func() error) {
		for _, i := range toSchedule {
			i, d := i, resolvingDeps[i]
			work <- func() error {
				if err := d.createIfNotExists(ctx, u.mutator, luciProject); err != nil {
					return err
				}
				if err := d.schedule(ctx, u, luciProject); err != nil {
					return err
				}
				ret[i] = d.resolvedDep
				return nil
			}
		}
	})
	if errs != nil {
		return nil, common.MostSevereError(err)
	}
	return sortDeps(ret), nil
}

///////////////////////////////////////////////////////////////////////////////
// implementation details.

func (u *Updater) handleBatch(ctx context.Context, batch *BatchUpdateCLTask) error {
	total := len(batch.GetTasks())
	err := parallel.WorkPool(min(16, total), func(work chan<- func() error) {
		for _, task := range batch.GetTasks() {
			task := task
			work <- func() error { return u.Schedule(ctx, task) }
		}
	})
	switch merrs, ok := err.(errors.MultiError); {
	case err == nil:
		return nil
	case !ok:
		return err
	default:
		failed, _ := merrs.Summary()
		err = common.MostSevereError(merrs)
		return errors.Annotate(err, "failed to schedule UpdateCLTask for %d out of %d CLs, keeping the most severe error", failed, total).Err()
	}
}

// TestingForceUpdate runs the CL Updater synchronously.
//
// For use in tests only. Production code should use Schedule() to benefit from
// task de-duplication.
//
// TODO(crbug/1284393): revisit the usefullness of the sync refresh after
// consistency-on-demand is provided by Gerrit.
func (u *Updater) TestingForceUpdate(ctx context.Context, task *UpdateCLTask) error {
	return u.handleCL(ctx, task)
}

func (u *Updater) handleCL(ctx context.Context, task *UpdateCLTask) error {
	if ignored_eids.Has(task.GetExternalId()) {
		logging.Debugf(ctx, "crbug/1290468: Ignore the request to update this large CL stack")
		return nil
	}
	cl, err := u.preload(ctx, task)
	if err != nil {
		return common.TQifyError(ctx, err)
	}
	// cl.ID == 0 means CL doesn't yet exist.
	ctx = logging.SetFields(ctx, logging.Fields{
		"project": task.GetLuciProject(),
		"id":      cl.ID,
		"eid":     cl.ExternalID,
	})

	backend, err := u.backendFor(cl)
	if err != nil {
		return common.TQifyError(ctx, err)
	}

	if err := u.handleCLWithBackend(ctx, task, cl, backend); err != nil {
		return backend.TQErrorSpec().Error(ctx, err)
	}
	return nil
}

func (u *Updater) handleCLWithBackend(ctx context.Context, task *UpdateCLTask, cl *CL, backend UpdaterBackend) error {
	// Save ID and ExternalID before giving CL to backend to avoid accidental corruption.
	id, eid := cl.ID, cl.ExternalID
	skip, updateFields, err := u.trySkippingFetch(ctx, task, cl, backend)
	switch {
	case err != nil:
		return err
	case !skip:
		updateFields, err = backend.Fetch(ctx, cl, task.GetLuciProject(), common.PB2TimeNillable(task.GetUpdatedHint()))
		if err != nil {
			return errors.Annotate(err, "%T.Fetch failed", backend).Err()
		}
	}

	if updateFields.IsEmpty() {
		logging.Debugf(ctx, "No update is necessary")
		return nil
	}

	// Transactionally update the CL.
	transClbk := func(latest *CL) error {
		if changed := updateFields.Apply(latest); !changed {
			// Someone, possibly even us in case of Datastore transaction retry, has
			// already updated this CL.
			return ErrStopMutation
		}
		return nil
	}
	if cl.ID == 0 {
		_, err = u.mutator.Upsert(ctx, task.GetLuciProject(), eid, transClbk)
	} else {
		_, err = u.mutator.Update(ctx, task.GetLuciProject(), id, transClbk)
	}
	return err
}

// trySkippingFetch checks if a fetch from the backend can be skipped.
//
// Returns true if so.
// NOTE: UpdateFields may be set if fetch can be skipped, meaning CL entity
// should be updated in Datastore.
func (u *Updater) trySkippingFetch(ctx context.Context, task *UpdateCLTask, cl *CL, backend UpdaterBackend) (bool, UpdateFields, error) {
	if cl.ID == 0 || cl.Snapshot == nil || cl.Snapshot.GetOutdated() != nil || task.GetUpdatedHint() == nil {
		return false, UpdateFields{}, nil
	}
	if task.GetUpdatedHint().AsTime().After(cl.Snapshot.GetExternalUpdateTime().AsTime()) {
		// There is no confidence that Snapshot is up-to-date, so proceed fetching
		// anyway.

		// NOTE: it's tempting to check first whether the LUCI project is watching
		// the CL given the existing Snapshot and skip the fetch if it's not the
		// case. However, for Gerrit CLs, the ref is mutable after the CL
		// creation and since ref is used to determine if CL is being watched,
		// we can't skip the fetch. For an example, see Gerrit move API
		// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#move-change
		return false, UpdateFields{}, nil
	}

	// CL Snapshot is up to date, but does it belong to the right LUCI project?
	acfg, err := backend.LookupApplicableConfig(ctx, cl)
	if err != nil {
		err = errors.Annotate(err, "%T.LookupApplicableConfig failed", backend).Err()
		return false, UpdateFields{}, err
	}
	if acfg == nil {
		// Insufficient saved CL, need to fetch before deciding if CL is watched.
		return false, UpdateFields{}, err
	}

	// Update CL with the new set of watching projects if materially different,
	// which should be saved to Datastore even if the fetch from Gerrit itself is
	// skipped.
	var toUpdate UpdateFields
	if !cl.ApplicableConfig.SemanticallyEqual(acfg) {
		toUpdate.ApplicableConfig = acfg
	}

	if !acfg.HasProject(task.GetLuciProject()) {
		// This project isn't watching the CL, so no need to fetch.
		//
		// NOTE: even if the Snapshot was fetched in the context of this project before,
		// we don't have to erase the Snapshot from the CL immediately: the update
		// in cl.ApplicableConfig suffices to ensure that CV won't be using the
		// Snapshot.
		return true, toUpdate, nil
	}

	if !acfg.HasProject(cl.Snapshot.GetLuciProject()) {
		// The Snapshot was previously fetched in the context of a project which is
		// no longer watching the CL.
		//
		// This can happen in practice in case of e.g. newly created "chromium-mXXX"
		// project to watch for a specific ref which was previously watched by a
		// generic "chromium" project. A Snapshot of a CL on such a ref would have
		// been fetched in the context of "chromium" first, and now it must be re-fetched
		// under "chromium-mXXX" to verify that the new project hasn't lost access
		// to the Gerrit CL.
		logging.Warningf(ctx, "Detected switch from %q LUCI project", cl.Snapshot.GetLuciProject())
		return false, toUpdate, nil
	}

	// At this point, these must be true:
	// * the Snapshot is up-to-date to the best of CV knowledge;
	// * this project is watching the CL, but there may be other projects, too;
	// * the Snapshot was created by a project still watching the CL, but which may
	//   differ from this project.
	if len(acfg.GetProjects()) >= 2 {
		// When there are several watching projects, projects shouldn't race
		// re-fetching & saving Snapshot. No new Runs are going to be started on
		// such CLs, so skip fetching new snapshot.
		return true, toUpdate, nil
	}

	// There is just 1 project, so check the invariant.
	if task.GetLuciProject() != cl.Snapshot.GetLuciProject() {
		panic(fmt.Errorf("BUG: this project %q must have created the Snapshot, not %q", task.GetLuciProject(), cl.Snapshot.GetLuciProject()))
	}

	if restriction := cl.Access.GetByProject()[task.GetLuciProject()]; restriction != nil {
		// For example, Gerrit has responded HTTP 403/404 before.
		// Must fetch again to verify if restriction still holds.
		logging.Debugf(ctx, "Detected prior access restriction: %s", restriction)
		return false, toUpdate, nil
	}

	// Finally, do refresh if the CL entity is just really old.
	if clock.Since(ctx, cl.UpdateTime) > autoRefreshAfter {
		// Strictly speaking, cl.UpdateTime isn't just changed on refresh, but also
		// whenever Run starts/ends. However, the start of Run is usually
		// happenening right after recent refresh, and end of Run is usually
		// followed by the refresh.
		return false, toUpdate, nil
	}

	// OK, skip the fetch.
	return true, toUpdate, nil
}

func (*Updater) preload(ctx context.Context, task *UpdateCLTask) (*CL, error) {
	if task.GetLuciProject() == "" {
		return nil, errors.New("invalid task input: LUCI project must be given")
	}
	eid := ExternalID(task.GetExternalId())
	id := common.CLID(task.GetId())
	switch {
	case id != 0:
		cl := &CL{ID: common.CLID(id)}
		switch err := datastore.Get(ctx, cl); {
		case err == datastore.ErrNoSuchEntity:
			return nil, errors.Annotate(err, "CL %d %q doesn't exist in Datastore", id, task.GetExternalId()).Err()
		case err != nil:
			return nil, errors.Annotate(err, "failed to load CL %d", id).Tag(transient.Tag).Err()
		case eid != "" && eid != cl.ExternalID:
			return nil, errors.Reason("invalid task input: CL %d actually has %q ExternalID, not %q", id, cl.ExternalID, eid).Err()
		default:
			return cl, nil
		}
	case eid == "":
		return nil, errors.Reason("invalid task input: either internal ID or ExternalID must be given").Err()
	default:
		switch cl, err := eid.Load(ctx); {
		case err != nil:
			return nil, errors.Annotate(err, "failed to load CL %q", eid).Tag(transient.Tag).Err()
		case cl == nil:
			// New CL to be created.
			return &CL{
				ExternalID: eid,
				ID:         0, // will be populated later.
				EVersion:   0,
			}, nil
		default:
			return cl, nil
		}
	}
}

func (u *Updater) backendFor(cl *CL) (UpdaterBackend, error) {
	kind, err := cl.ExternalID.kind()
	if err != nil {
		return nil, err
	}
	u.rwmutex.RLock()
	defer u.rwmutex.RUnlock()
	if b, exists := u.backends[kind]; exists {
		return b, nil
	}
	return nil, errors.Reason("%q backend is not supported", kind).Err()
}

// makeTaskDeduplicationKey returns TQ task deduplication key.
func makeTaskDeduplicationKey(ctx context.Context, t *UpdateCLTask, delay time.Duration) string {
	// Dedup in the short term to avoid excessive number of refreshes,
	// but ensure eventually calling Schedule with the same payload results in a
	// new task. This is done by de-duping only within a single "epoch" window,
	// which differs by CL to avoid synchronized herd of requests hitting
	// a backend (e.g. Gerrit).
	//
	// +----------------------------------------------------------------------+
	// |                 ... -> time goes forward -> ....                     |
	// +----------------------------------------------------------------------+
	// |                                                                      |
	// | ... | epoch (N-1, CL-A) | epoch (N, CL-A) | epoch (N+1, CL-A) | ...  |
	// |                                                                      |
	// |            ... | epoch (N-1, CL-B) | epoch (N, CL-B) | ...           |
	// +----------------------------------------------------------------------+
	//
	// Furthermore, de-dup window differs based on wheter updatedHint is given
	// or it's a blind refresh.
	interval := blindRefreshInterval
	if t.GetUpdatedHint() != nil {
		interval = knownRefreshInterval
	}
	// Prefer ExternalID if both ID and ExternalID are known, as the most frequent
	// use-case for update via PubSub/Polling, which specifies ExternalID and may
	// not resolve it to internal ID just yet.
	uniqArg := t.GetExternalId()
	if uniqArg == "" {
		uniqArg = strconv.FormatInt(t.GetId(), 16)
	}
	epochOffset := common.DistributeOffset(interval, "update-cl", t.GetLuciProject(), uniqArg)
	epochTS := clock.Now(ctx).Add(delay).Truncate(interval).Add(interval + epochOffset)

	var sb strings.Builder
	sb.WriteString("v0")
	sb.WriteRune('\n')
	sb.WriteString(t.GetLuciProject())
	sb.WriteRune('\n')
	sb.WriteString(uniqArg)
	_, _ = fmt.Fprintf(&sb, "\n%x", epochTS.UnixNano())
	if h := t.GetUpdatedHint(); h != nil {
		_, _ = fmt.Fprintf(&sb, "\n%x", h.AsTime().UnixNano())
	}
	return sb.String()
}

// makeTQTitleForHumans makes human-readable TQ task title.
//
// WARNING: do not use for anything else. Doesn't guarantee uniqueness.
//
// It'll be visible in logs as the suffix of URL in Cloud Tasks console
// and in the GAE requests log.
//
// The primary purpose is that quick search for specific CL in the GAE request
// log alone, as opposed to searching through much larger and separate stderr
// log of the process (which is where logging.Logf calls go into).
//
// For example, the title for the task with all the field specified:
//   "proj/gerrit/chromium/1111111/u2016-02-03T04:05:06Z"
func makeTQTitleForHumans(t *UpdateCLTask) string {
	var sb strings.Builder
	sb.WriteString(t.GetLuciProject())
	if id := t.GetId(); id != 0 {
		_, _ = fmt.Fprintf(&sb, "/%d", id)
	}
	if eid := t.GetExternalId(); eid != "" {
		sb.WriteRune('/')
		// Reduce verbosity in common case of Gerrit on googlesource.
		// Although it's possible to delegate this to backend, the additional
		// boilerplate isn't yet justified.
		if kind, err := ExternalID(eid).kind(); err == nil && kind == "gerrit" {
			eid = strings.Replace(eid, "-review.googlesource.com/", "/", 1)
		}
		sb.WriteString(eid)
	}
	if h := t.GetUpdatedHint(); h != nil {
		sb.WriteString("/u")
		sb.WriteString(h.AsTime().UTC().Format(time.RFC3339))
	}
	return sb.String()
}

func resolveDeps(ctx context.Context, luciProject string, deps map[ExternalID]DepKind) ([]resolvingDep, error) {
	eids := make([]ExternalID, 0, len(deps))
	ret := make([]resolvingDep, 0, len(deps))
	for eid, kind := range deps {
		eids = append(eids, eid)
		ret = append(ret, resolvingDep{eid: eid, kind: kind})
	}

	ids, err := Lookup(ctx, eids)
	if err != nil {
		return nil, err
	}
	cls := make([]*CL, 0, len(deps))
	for i, id := range ids {
		if id > 0 {
			cls = append(cls, &CL{ID: id})
			ret[i].resolvedDep = &Dep{Clid: int64(id), Kind: ret[i].kind}
		}
	}
	if len(cls) == 0 {
		return ret, nil
	}

	if len(cls) > 500 {
		// This may need optimizing if CV starts handling CLs with 1k dep to load CLs
		// in batches and avoid excessive RAM usage.
		logging.Warningf(ctx, "Loading %d CLs (deps) at once may lead to excessive RAM usage", len(cls))
	}
	if _, err := loadCLs(ctx, cls); err != nil {
		return nil, err
	}
	// Must iterate `ids` since not every id has an entry in `cls`.
	for i, id := range ids {
		if id == 0 {
			continue
		}
		cl := cls[0]
		cls = cls[1:]
		if !depNeedsRefresh(ctx, cl, luciProject) {
			ret[i].ready = true
		}
	}
	return ret, nil
}

// resolvingDep represents a dependency known by its external ID only being
// resolved.
//
// Helper struct for the Updater.ResolveAndScheduleDeps.
type resolvingDep struct {
	eid         ExternalID
	kind        DepKind
	ready       bool // true if already up to date and .dep is populated.
	resolvedDep *Dep // if nil, use createIfNotExists() to populate
}

func (d *resolvingDep) createIfNotExists(ctx context.Context, m *Mutator, luciProject string) error {
	if d.resolvedDep != nil {
		return nil // already exists
	}
	cl, err := m.Upsert(ctx, luciProject, d.eid, func(cl *CL) error {
		// TODO: somehow record when CL was inserted to put a boundary on how long
		// Project Manager should be waiting for the dep to be actually fetched &
		// its entity updated in Datastore.
		if cl.EVersion > 0 {
			// If CL already exists, we don't need to modify it % above comment.
			return ErrStopMutation
		}
		return nil
	})
	if err != nil {
		return err
	}
	d.resolvedDep = &Dep{Clid: int64(cl.ID), Kind: d.kind}
	return nil
}

func (d *resolvingDep) schedule(ctx context.Context, u *Updater, luciProject string) error {
	return u.Schedule(ctx, &UpdateCLTask{
		ExternalId:  string(d.eid),
		Id:          d.resolvedDep.GetClid(),
		LuciProject: luciProject,
	})
}

// sortDeps sorts given slice by CLID ASC in place and returns it.
func sortDeps(deps []*Dep) []*Dep {
	sort.Slice(deps, func(i, j int) bool {
		return deps[i].GetClid() < deps[j].GetClid()
	})
	return deps
}

// depNeedsRefresh returns true if the dependency CL needs a refresh in the
// context of a specific LUCI project.
func depNeedsRefresh(ctx context.Context, dep *CL, luciProject string) bool {
	switch {
	case dep == nil:
		panic(fmt.Errorf("dep CL must not be nil"))
	case dep.Snapshot == nil:
		return true
	case dep.Snapshot.GetOutdated() != nil:
		return true
	case dep.Snapshot.GetLuciProject() != luciProject:
		return true
	case clock.Since(ctx, dep.UpdateTime) > autoRefreshAfter:
		// Strictly speaking, cl.UpdateTime isn't just changed on refresh, but also
		// whenever Run starts/ends. However, the start of Run is usually
		// happenening right after recent refresh, and end of Run is usually
		// followed by the refresh.
		return true
	default:
		return false
	}
}
