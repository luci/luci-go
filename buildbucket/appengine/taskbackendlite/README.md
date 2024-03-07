# Buildbucket TaskbackendLite Service
Buildbucket TaskbackendLite Service is an implementation of
`service TaskbackendLite` in Buildbucket
[backend.proto](https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/buildbucket/proto/backend.proto#329).
The initial idea is to assist Google3 users of Buildbucket V1 who run builds on
their own backend to be able to migrate to Buildbucket V2 (see go/webrtc-bb-taskbackend).
This service acts as a simple bridge, receiving Buildbucket's RunTask requests
and publishing task notifications to the corresponding Cloud PubSub topic.

The task notification pubsub message schema is:
```
{
    "build_id": <string>,
    "start_build_token": <string>,
}
```
Attributes:
 - "dummy_task_id" (to deduplicate the message on the subscriber side)
 - "project"
 - "bucket"
 - "builder"


## How to onboard

To begin using this service, please get in touch with the service
[owners](https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/buildbucket/OWNERS).
This is to ensure that the dedicated PubSub topic ID (in the format"taskbackendlite-${project}")
has been set up for your project, and its corresponding LUCI project account -
(${project}-scoped@luci-project-accounts.iam.gserviceaccount.com) has been
granted the publisher permission.
