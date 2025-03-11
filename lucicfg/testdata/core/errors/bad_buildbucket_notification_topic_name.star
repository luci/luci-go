luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
)

luci.buildbucket_notification_topic(
    name = "my-topic1",
)
# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bad_buildbucket_notification_topic_name.star: in <toplevel>
#   <builtin>: in _buildbucket_notification_topic
#   @stdlib//internal/validate.star: in _string
#   <builtin>: in fail
# Error: bad "name": "my-topic1" should match "^projects/[^/]+/topics/[^/]+$"
