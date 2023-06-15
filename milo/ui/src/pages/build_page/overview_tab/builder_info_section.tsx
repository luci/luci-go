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

import { GrpcError } from '@chopsui/prpc-client';

import { POTENTIAL_PERM_ERROR_CODES } from '@/common/constants';
import { usePrpcQuery } from '@/common/libs/use_prpc_query';
import { BuilderID, BuildersService } from '@/common/services/buildbucket';

export interface BuilderInfoSectionProps {
  readonly builderId: BuilderID;
}

export function BuilderInfoSection({ builderId }: BuilderInfoSectionProps) {
  const { data, error } = usePrpcQuery({
    host: CONFIGS.BUILDBUCKET.HOST,
    Service: BuildersService,
    method: 'getBuilder',
    request: {
      id: builderId,
    },
  });

  if (
    error &&
    !(
      error instanceof GrpcError &&
      POTENTIAL_PERM_ERROR_CODES.includes(error.code)
    )
  ) {
    // Optional resource.
    // Log the warning in case of an error.
    console.warn('failed to get builder description', error);
  }

  if (!data?.config.descriptionHtml) {
    return <></>;
  }

  return (
    <>
      <h3>Builder Info</h3>
      <div
        id="builder-description"
        dangerouslySetInnerHTML={{ __html: data.config.descriptionHtml }}
      />
    </>
  );
}
