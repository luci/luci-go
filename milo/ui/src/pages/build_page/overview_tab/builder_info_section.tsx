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

import { observer } from 'mobx-react-lite';

import { unwrapOrElse } from '../../../libs/utils';
import { useStore } from '../../../store';

export const BuilderInfoSection = observer(() => {
  const store = useStore();
  const pageState = store.buildPage;

  const builderDescriptionHtml = unwrapOrElse(
    () => {
      if (!pageState.canReadFullBuild) {
        return '';
      }
      return pageState.builder?.config.descriptionHtml || '';
    },
    (err) => {
      // The builder config might've been deleted from buildbucket or the
      // builder is a dynamic builder.
      console.warn('failed to get builder description', err);
      return '';
    }
  );
  if (!builderDescriptionHtml) {
    return <></>;
  }

  return (
    <>
      <h3>Builder Info</h3>
      <div id="builder-description" dangerouslySetInnerHTML={{ __html: builderDescriptionHtml }} />
    </>
  );
});
