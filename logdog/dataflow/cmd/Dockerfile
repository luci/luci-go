FROM gcr.io/dataflow-templates-base/go-template-launcher-base

ARG WORKDIR=/dataflow/template
RUN mkdir -p ${WORKDIR}

COPY backfill_expire_at ${WORKDIR}/backfill_expire_at

ENV FLEX_TEMPLATE_GO_BINARY="${WORKDIR}/backfill_expire_at"

ENTRYPOINT ["/opt/google/dataflow/go_template_launcher"]
