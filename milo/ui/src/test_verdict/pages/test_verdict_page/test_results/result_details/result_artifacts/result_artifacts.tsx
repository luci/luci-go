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
import { TextField } from '@mui/material';
import Accordion from '@mui/material/Accordion';
import AccordionDetails from '@mui/material/AccordionDetails';
import AccordionSummary from '@mui/material/AccordionSummary';
import Grid from '@mui/material/Grid';
import Typography from '@mui/material/Typography';
import { useEffect, useMemo, useState } from 'react';
import { useDebounce } from 'react-use';

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

function filterArtifacts(searchTerm: string, artifacts: Artifact[]) {
  if (!searchTerm) {
    return artifacts;
  }
  return artifacts.filter((artifact) =>
    artifact.artifactId.includes(searchTerm),
  );
}

interface SearchInputProps {
  label: string;
  onSearchTermChange: (term: string) => void;
}
// This component ensures that when the input changes
// only this component rerenders.
function SearchInput({ label, onSearchTermChange }: SearchInputProps) {
  const [searchTerm, setSearchTerm] = useState('');
  useDebounce(() => onSearchTermChange(searchTerm), 200, [searchTerm]);
  return (
    <TextField
      value={searchTerm}
      onChange={(e) => setSearchTerm(e.target.value)}
      size="small"
      label={label}
      fullWidth
      data-testid={label}
    />
  );
}

interface ArtifactAccordionProps {
  header: string;
  searchLabel: string;
  processedArtifacts: ProcessedArtifacts;
}

function ArtifactsAccordion({
  header,
  searchLabel,
  processedArtifacts,
}: ArtifactAccordionProps) {
  const [expanded, setExpanded] = useState(false);
  const [currentTerm, setCurrentTerm] = useState('');
  const [filteredLinkArtifacts, setFilteredLinkArtifacts] = useState<
    Artifact[]
  >([]);

  useEffect(() => {
    setFilteredLinkArtifacts(filterArtifacts(currentTerm, processedArtifacts.links));
  }, [processedArtifacts.links, currentTerm]);

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
      <AccordionSummary
        sx={{
          display: 'flex',
          alignItems: 'center',
        }}
        expandIcon={<ExpandMoreIcon />}
      >
        <Typography>{header}</Typography>
      </AccordionSummary>
      <AccordionDetails>
        <Grid
          item
          container
          xs={10}
          sx={{
            py: 2,
          }}
        >
          <SearchInput
            label={searchLabel}
            onSearchTermChange={(term) =>
              setCurrentTerm(term)
            }
          />
        </Grid>
        <Grid
          container
          direction="row"
          sx={{
            maxHeight: '500px',
            overflowY: 'auto',
          }}
        >
          <Grid container direction="column" rowSpacing="5">
            {filteredLinkArtifacts.map((artifact) => (
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
        header={`Result artifacts ${resultArtifacts.length}`}
        searchLabel="Search result artifacts"
      />
      <ArtifactsAccordion
        processedArtifacts={processedInvArtifacts}
        header={`Invocation artifacts ${invArtifacts.length}`}
        searchLabel="Search invocation artifacts"
      />
    </>
  );
}
