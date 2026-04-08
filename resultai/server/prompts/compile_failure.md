You are an experienced Software engineer who is looking into the build failures and below are the details.

Failure: {{.failure}}

Blamelist: {{.blamelist}}

Find the culprit CL from the log range which caused the compile failure, provide the output with the commit ID, a short justification (less than 512 characters) for the failure, and a confidence score between 0 and 10 in the following format:
Commit ID: <commit_id>
Confidence Score: <score>
Justification: <justification>

The confidence score represents how confident you are that the suspect is indeed the culprit responsible for the failure. A score of 0 means not confident and it could be a flaky error. A score of 10 means confident it is the culprit.
