// Copyright 2025 The LUCI Authors.
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

import { Box, Chip, Typography } from '@mui/material';

import {
  AccountInfo,
  ChangeMessageInfo,
  GerritChangeInfo,
  gerritChangeInfo_StatusToJSON,
} from '@/proto/turboci/data/gerrit/v1/gerrit_change_info.pb';
import {
  GobSourceCheckOptions,
  GobSourceCheckOptions_PinnedRepoMounts_GitCommit,
} from '@/proto/turboci/data/gerrit/v1/gob_source_check_options.pb';
import { GobSourceCheckResults } from '@/proto/turboci/data/gerrit/v1/gob_source_check_results.pb';

import { DetailRow } from './detail_row';
import { GenericJsonDetails } from './generic_json_details';

export function GobSourceCheckOptionsDetails({
  data,
}: {
  data: GobSourceCheckOptions;
}) {
  return (
    <Box>
      {data.gerritChanges.length > 0 && (
        <Box sx={{ mb: 2 }}>
          <Typography variant="caption">Gerrit Changes</Typography>
          {data.gerritChanges.map((change, idx) => (
            <Box
              key={idx}
              sx={{
                p: 1,
                border: '1px solid #eee',
                borderRadius: 1,
                mt: 0.5,
              }}
            >
              <DetailRow label="Host" value={change.hostname} />
              <DetailRow label="Change" value={change.changeNumber} />
              <DetailRow label="Patchset" value={change.patchset} />
              {change.mountsToApply.length > 0 && (
                <DetailRow
                  label="Mounts"
                  value={change.mountsToApply.join(', ')}
                />
              )}
            </Box>
          ))}
        </Box>
      )}
      {data.basePinnedRepos && (
        <Box>
          <Typography variant="caption">Base Pinned Repos</Typography>
          <Box
            sx={{ p: 1, border: '1px solid #eee', borderRadius: 1, mt: 0.5 }}
          >
            {data.basePinnedRepos.projectCommit && (
              <Box sx={{ mb: 1 }}>
                <Typography variant="caption">Project Commit</Typography>
                <Box sx={{ pl: 1 }}>
                  <GitCommitDetails data={data.basePinnedRepos.projectCommit} />
                </Box>
              </Box>
            )}
            {data.basePinnedRepos.manifestCommit && (
              <Box sx={{ mb: 1 }}>
                <Typography variant="caption">Manifest Commit</Typography>
                <Box sx={{ pl: 1, mt: 0.5, borderLeft: '2px solid #eee' }}>
                  {data.basePinnedRepos.manifestCommit.commit && (
                    <GitCommitDetails
                      data={data.basePinnedRepos.manifestCommit.commit}
                    />
                  )}
                  <DetailRow
                    label="Manifest Path"
                    value={data.basePinnedRepos.manifestCommit.path}
                  />
                </Box>
              </Box>
            )}
            {data.basePinnedRepos.mountOverrides.length > 0 && (
              <Box>
                <Typography variant="caption">Mount Overrides</Typography>
                {data.basePinnedRepos.mountOverrides.map((override, idx) => (
                  <Box
                    key={idx}
                    sx={{ pl: 1, mt: 0.5, borderLeft: '2px solid #eee' }}
                  >
                    <DetailRow label="Mount Path" value={override.mount} />
                    {override.override && (
                      <GitCommitDetails data={override.override} />
                    )}
                  </Box>
                ))}
              </Box>
            )}
          </Box>
        </Box>
      )}
    </Box>
  );
}

function GitCommitDetails({
  data,
}: {
  data: GobSourceCheckOptions_PinnedRepoMounts_GitCommit;
}) {
  return (
    <>
      <DetailRow label="Git Host" value={data.host} />
      <DetailRow label="Git Project" value={data.project} />
      <DetailRow label="Git Ref" value={data.ref} />
      <DetailRow label="Git ID" value={data.id} />
    </>
  );
}

export function GobSourceCheckResultsDetails({
  data,
}: {
  data: GobSourceCheckResults;
}) {
  return (
    <Box>
      {data.changes.map((change, idx) => (
        <Box
          key={change.changeId || idx}
          sx={{
            '&:not(:last-child)': {
              mb: 2,
              borderBottom: '2px solid #eee',
              pb: 2,
            },
          }}
        >
          <GerritChangeInfoDetails data={change} />
        </Box>
      ))}
    </Box>
  );
}

function GerritChangeInfoDetails({ data }: { data: GerritChangeInfo }) {
  return (
    <Box>
      <DetailRow label="Host" value={data.host} />
      <DetailRow label="Project" value={data.project} />
      <DetailRow label="Branch" value={data.branch} />
      <DetailRow label="Change Number" value={data.changeNumber} />
      <DetailRow label="Patchset" value={data.patchset} />
      <DetailRow
        label="Status"
        value={
          data.status ? gerritChangeInfo_StatusToJSON(data.status) : 'UNKNOWN'
        }
      />
      <DetailRow label="Topic" value={data.topic} />
      <DetailRow label="Change-Id" value={data.changeId} />
      {data.owner && (
        <DetailRow
          label="Owner"
          value={<AccountInfoDetails data={data.owner} />}
        />
      )}
      <DetailRow label="Created" value={data.creationTime} />
      <DetailRow label="Updated" value={data.lastModificationTime} />
      {data.submittedTime && (
        <DetailRow label="Submitted" value={data.submittedTime} />
      )}

      {/* Just JSON.stringify deeply-nested fields until we have a reason
      to render them differently */}
      <Box sx={{ mt: 1 }}>
        <GenericJsonDetails
          json={JSON.stringify(data.labels)}
          typeUrl="Labels"
        />
      </Box>
      <Box sx={{ mt: 1 }}>
        <GenericJsonDetails
          json={JSON.stringify(data.reviewers)}
          typeUrl="Reviewers"
        />
      </Box>
      <Box sx={{ mt: 1 }}>
        <GenericJsonDetails
          json={JSON.stringify(data.revisions)}
          typeUrl="Revisions"
        />
      </Box>
      {data.messages.length > 0 && (
        <Box sx={{ mt: 2 }}>
          <Typography variant="caption">
            Messages ({data.messages.length})
          </Typography>
          <Box sx={{ maxHeight: 200, overflowY: 'auto', mt: 0.5 }}>
            {data.messages.map((msg, idx) => (
              <ChangeMessageDetails key={msg.id || idx} data={msg} />
            ))}
          </Box>
        </Box>
      )}
    </Box>
  );
}

function AccountInfoDetails({ data }: { data: AccountInfo }) {
  let primary = data.displayName || data.name || data.username;
  if (data.email) {
    primary += ` <${data.email}>`;
  }
  return (
    <Typography variant="body2">
      {primary} {data.accountId && `(${data.accountId})`}
    </Typography>
  );
}

function ChangeMessageDetails({ data }: { data: ChangeMessageInfo }) {
  return (
    <Box
      sx={{
        borderLeft: '2px solid #eee',
        pl: 1,
        mb: 1,
        fontSize: '0.8rem',
      }}
    >
      <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
        {data.author && (
          <Typography variant="caption">
            {data.author.name || data.author.email}
          </Typography>
        )}
        <Typography variant="caption" color="text.secondary">
          {data.date}
        </Typography>
        {data.patchset && (
          <Chip
            size="small"
            label={`PS ${data.patchset}`}
            sx={{ height: 16, fontSize: '0.65rem' }}
          />
        )}
        {data.tag && (
          <Chip
            size="small"
            label={data.tag}
            sx={{ height: 16, fontSize: '0.65rem' }}
          />
        )}
      </Box>
      <Typography
        variant="body2"
        sx={{ whiteSpace: 'pre-wrap', fontSize: 'inherit', mt: 0.5 }}
      >
        {data.message}
      </Typography>
    </Box>
  );
}
