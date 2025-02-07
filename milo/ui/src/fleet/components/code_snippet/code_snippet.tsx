import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { Button, Paper, Tooltip } from '@mui/material';
import Grid from '@mui/material/Grid2';
import Item from '@mui/material/Grid2';
import { Box } from '@mui/system';

interface CodeSnippetProps {
  copyText: string;
  displayText?: string;
}

export default function CodeSnippet({
  copyText,
  displayText,
}: CodeSnippetProps) {
  const handleCopy = async () => {
    await navigator.clipboard.writeText(copyText);
  };

  return (
    <Paper elevation={0} variant="outlined">
      <Grid
        container
        direction="row"
        justifyContent="flex-end"
        alignItems="center"
      >
        <Grid size={11}>
          <Item>
            <Box sx={{ p: '20px' }}>
              <code>{displayText ?? copyText}</code>
            </Box>
          </Item>
        </Grid>
        <Grid size={1}>
          <Item>
            <Tooltip title="Copy to clipboard">
              <Button
                onClick={handleCopy}
                sx={{ minWidth: '30px' }}
                aria-label="Copy to clipboard"
              >
                <ContentCopyIcon sx={{ height: '20px' }} />
              </Button>
            </Tooltip>
          </Item>
        </Grid>
      </Grid>
    </Paper>
  );
}
