luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
)

luci.buildbucket_notification_topic(
    name = "projects/my-cloud-project2/topics/my-topic2",
    compression = "any",
)
# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bad_buildbucket_notification_topic_compression.star: in <toplevel>
#   <builtin>: in _buildbucket_notification_topic
#   <builtin>: in fail
# Error: bad "compression" value. It must be in ["ZLIB", "ZSTD"]
