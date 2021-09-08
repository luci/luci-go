# CV PubSub Messages

The `pubsub.proto` includes the proto messages of the PubSub topics, where CV
publishes messages for CV run events.

## Subscribing to CV topics

Currently, access to all the CV topics are protected by ACL. To subscribe
to CV topics, contact [g/luci-eng] or file a bug at [go/luci-bug-admin] to
request access to the CV topics.

[g/luci-eng]: http://g/luci-eng
[go/luci-bug-admin]: http://go/luci-bug-admin

## Topics

NOTE: Due to the nature of Cloud PubSub, duplicate messages may be delivered
for a single Run event. However, it's guaranteed that at least one message will
be delivered for each Run event.

NOTE: Message formats can be updated with additional fields in the future,
and all messages are encoded in [JSONPB]. Ignore unknown fields when decoding
the payload to avoid breakages.

[JSONPB]: https://developers.google.com/protocol-buffers/docs/proto3#json).

### RunEnded
- Topic: `projects/luci-change-verifier/topics/v1.run_ended`
- Message Format: https://pkg.go.dev/go.chromium.org/luci/cv/api/v1#PubSubRun
- Attributes:
  - `luci_project`: the LUCI project the CV Run belongs to.
  - `status`: the terminal status of the CV Run. Visit [RunStatus] for a list of
    terminal statuses.
- Description: When a CV Run changes to a terminal status, RunEnded event will
  be published into this topic, and the status field tells the reason of the
  termination.

[RunStatus]: https://pkg.go.dev/go.chromium.org/luci/cv/api/v1#Run_Status

## Adding a new topic

1. Update the terraform config to create a new topic.
2. Add a new proto message for the topic in this folder, if necessary.
3. Register a new TaskClass and add utility functions for the new topic in
cv/internal/run/pubsub.
4. Update this doc with the new topic.

