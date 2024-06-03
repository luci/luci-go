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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import Accordion from '@mui/material/Accordion';
import AccordionDetails from '@mui/material/AccordionDetails';
import AccordionSummary from '@mui/material/AccordionSummary';
import Grid from '@mui/material/Grid';
import Typography from '@mui/material/Typography';
import { useMemo, useState } from 'react';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { ArtifactLink } from '@/test_verdict/components/artifact_link';

import { useInvArtifacts, useResultArtifacts } from '../context';

interface ProcessedArtifacts {
  textDiff?: Artifact;
  imageDiffGroup: {
    expected?: Artifact;
    actual?: Artifact;
    diff?: Artifact;
  };
  links: Artifact[];
}

function processArtifacts(artifacts: readonly Artifact[]): ProcessedArtifacts {
  const result: ProcessedArtifacts = {
    imageDiffGroup: {},
    links: [],
  };
  artifacts.forEach((artifact) => {
    if (artifact.artifactId === 'text_diff') {
      result.textDiff = artifact;
    } else if (artifact.artifactId === 'expected_image') {
      result.imageDiffGroup!.expected = artifact;
    } else if (artifact.artifactId === 'actual_image') {
      result.imageDiffGroup!.actual = artifact;
    } else if (artifact.artifactId === 'image_diff') {
      result.imageDiffGroup!.diff = artifact;
    } else {
      result.links.push(artifact);
    }
  });

  return result;
}

interface ArtifactAccordionProps {
  title: string;
  processedArtifacts: ProcessedArtifacts;
}

function ArtifactsAccordion({
  title,
  processedArtifacts,
}: ArtifactAccordionProps) {
  const [expanded, setExpanded] = useState(false);

  function handleExpandedClicked(isExpanded: boolean) {
    setExpanded(isExpanded);
  }

  return (
    <Accordion
      variant="outlined"
      disableGutters
      expanded={expanded}
      onChange={() => handleExpandedClicked(!expanded)}
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography sx={{ mr: 1 }}>{title}</Typography>
      </AccordionSummary>
      <AccordionDetails>
        <Grid
          container
          direction="row"
          rowSpacing="5"
          sx={{
            maxHeight: '400px',
            overflowY: 'scroll',
          }}
        >
          <Grid container direction="column" rowSpacing="5">
            {processedArtifacts.links.map((artifact) => (
              <Grid item key={artifact.artifactId}>
                <ArtifactLink artifact={artifact} />
              </Grid>
            ))}
          </Grid>
        </Grid>
      </AccordionDetails>
    </Accordion>
  );
}

export function ResultArtifacts() {
  const resultArtifacts = useResultArtifacts();
  const invArtifacts = useInvArtifacts();
  const processedResultArtifacts = useMemo(
    () => processArtifacts(resultArtifacts),
    [resultArtifacts],
  );

  const processedInvArtifacts = useMemo(
    () => processArtifacts(invArtifacts),
    [invArtifacts],
  );

  return (
    <>
      <ArtifactsAccordion
        processedArtifacts={processedResultArtifacts}
        title={`Result artifacts ${resultArtifacts.length}`}
      />
      <ArtifactsAccordion
        processedArtifacts={processedInvArtifacts}
        title={`Invocation artifacts ${invArtifacts.length}`}
      />
    </>
  );
}
