// Copyright 2024 The LUCI Authors.
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

package dsutils

import (
	"context"
	"fmt"

	cloudds "cloud.google.com/go/datastore"
	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/log"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/register"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/errors"
)

func init() {
	register.DoFn3x1(&getAllNamespacesFn{})
	register.Emitter1[string]()
}

// GetAllNamespaces queries all the namespaces from datastore and returns
// PCollection<string>.
func GetAllNamespaces(s beam.Scope, project string) beam.PCollection {
	s = s.Scope(fmt.Sprintf("datastore.GetAllNamespaces.%s", project))
	imp := beam.Impulse(s)
	return beam.ParDo(s, &getAllNamespacesFn{Project: project}, imp)
}

type getAllNamespacesFn struct {
	Project string
	client  clientType
}

type clientType interface {
	Run(context.Context, *cloudds.Query) *cloudds.Iterator
}

func (fn *getAllNamespacesFn) Setup(ctx context.Context) error {
	if fn.client == nil {
		client, err := cloudds.NewClient(ctx, fn.Project)
		if err != nil {
			return errors.Annotate(err, "failed to construct cloud datastore client").Err()
		}
		fn.client = client
	}
	return nil
}

func (fn *getAllNamespacesFn) ProcessElement(ctx context.Context, _ []byte, emit func(string)) error {
	i := 0
	iter := fn.client.Run(ctx, cloudds.NewQuery("__namespace__").KeysOnly())
	for {
		k, err := iter.Next(nil)
		if err != nil {
			if errors.Is(err, iterator.Done) {
				break
			}
			return err
		}
		log.Infof(ctx, "Datastore: emitting namespace #%d: `%s`", i, k.Name)
		emit(k.Name)
		i++
	}

	return nil
}
