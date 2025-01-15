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

import Link from '@mui/material/Link';
import { useQuery } from '@tanstack/react-query';

import { useMiloInternalClient } from '@/common/hooks/prpc_clients';
import { StringPair } from '@/common/services/common';
import { TestMetadata } from '@/common/services/resultdb';
import { getCodeSourceUrl } from '@/common/tools/url_utils';
import { extractProject } from '@/common/tools/utils';
import { Project } from '@/proto/go.chromium.org/luci/milo/proto/projectconfig/project.pb';
import { QueryTestMetadataRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestLocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_metadata.pb';

import { useTestMetadata } from './utils';

export interface TestIdLabelProps {
  readonly projectOrRealm: string; // A project name or a realm name.
  readonly testId: string;
}

function propertiesToDisplay(
  projectCfg: Project,
  testMetadata: TestMetadata,
): StringPair[] {
  const propertiesCfgs = projectCfg.metadataConfig?.testMetadataProperties;
  // Return empty if no test metadata properties or, test metadata properties schema or display config is unspecified.
  if (
    !testMetadata.properties ||
    !testMetadata.propertiesSchema ||
    !propertiesCfgs
  ) {
    return [];
  }
  // Find the right display rules to apply to the test metadata properties based on the schema name.
  const propertiesCfg = propertiesCfgs.find(
    (t) => t.schema === testMetadata.propertiesSchema,
  );
  if (!propertiesCfg) {
    return [];
  }
  const pairs: StringPair[] = [];
  for (const item of propertiesCfg.displayItems) {
    const value = extractProperty(testMetadata.properties, item.path);
    if (value !== undefined && typeof value === 'string') {
      pairs.push({ key: item.displayName, value });
    }
  }
  return pairs;
}

type PropertyObject = { [key: string]: unknown };
function extractProperty(
  properties: PropertyObject | undefined,
  path: string,
): string | undefined {
  if (!properties) return undefined;
  let current: PropertyObject | undefined = properties;
  for (const part of path.split('.')) {
    current = current[part] as PropertyObject | undefined;
    if (current === undefined) return undefined;
  }
  return current.toString();
}

export function TestIdLabel({ projectOrRealm, testId }: TestIdLabelProps) {
  const project = extractProject(projectOrRealm);
  const {
    data: testMetadataDetail,
    isSuccess: tmIsSuccess,
    isLoading: tmIsLoading,
  } = useTestMetadata(
    QueryTestMetadataRequest.fromPartial({
      project,
      predicate: { testIds: [testId] },
    }),
  );
  const metadata = testMetadataDetail?.testMetadata;
  const testLocation = metadata?.location;
  const sourceURL = testLocation
    ? getCodeSourceUrl(
        TestLocation.fromPartial(testLocation),
        testMetadataDetail?.sourceRef?.gitiles?.ref,
      )
    : null;

  const client = useMiloInternalClient();
  const {
    data: projectCfg,
    isSuccess: cfgIsSuccess,
    isLoading: cfgIsLoading,
  } = useQuery(client.GetProjectCfg.query({ project }));

  return (
    <table>
      <tbody>
        <tr>
          <td
            css={{
              color: 'var(--light-text-color)',
              /* Shrink the first column */
              width: '0px',
            }}
          >
            Project
          </td>
          <td>{project}</td>
        </tr>
        <tr>
          <td css={{ color: 'var(--light-text-color)' }}>ID</td>
          <td>{testId}</td>
        </tr>
        {!tmIsLoading && tmIsSuccess && metadata && (
          <>
            {metadata.name && (
              <tr>
                <td css={{ color: 'var(--light-text-color)' }}>Test</td>
                {sourceURL ? (
                  <td>
                    <Link href={sourceURL} target="_blank">
                      {metadata.name}
                    </Link>
                  </td>
                ) : (
                  <td>{metadata.name}</td>
                )}
              </tr>
            )}
            {!cfgIsLoading &&
              cfgIsSuccess &&
              propertiesToDisplay(projectCfg, metadata).map((item, idx) => (
                <tr key={idx}>
                  <td css={{ color: 'var(--light-text-color)' }}>{item.key}</td>
                  <td>{item.value}</td>
                </tr>
              ))}
          </>
        )}
      </tbody>
    </table>
  );
}
