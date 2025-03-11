luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "ci")

luci.builder(
    name = "c",
    bucket = "ci",
    service_account = "noop@example.com",
    executable = "recipe",
    contact_team_email = "d@gooble",
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/builder_with_invalid_contact_email.star: in <toplevel>
#   ...
# Error: bad "contact_team_email": "d@gooble" is not a valid RFC 2822 email
