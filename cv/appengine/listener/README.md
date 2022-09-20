# Listener

Listener is responsible for listening to external services to find event
occurrences on the external resources that CV watches, such as Gerrit
changelist(s) and Buildbucket build(s). It enqueues TQ messages to trigger
event handlers on other services for certain event types.
