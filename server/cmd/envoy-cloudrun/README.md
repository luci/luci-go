# Generic Envoy for Cloud Run services

For Cloud Run services in
[Multi-Container](https://cloud.google.com/run/docs/deploying#sidecars)
architecture, this directory contains a generic Envoy proxy config and a
dockerfile to build a container. It enables the ability to serve both HTTP/1
traffic and gRPC traffic on the same Cloud Run service with envoy at the front
routing the traffic to the correct port.

Note: The application container(s) should expose HTTP/1 port on 8081 and gRPC
port on 8082.
