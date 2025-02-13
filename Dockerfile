FROM golang:1.23 AS build

WORKDIR /go/src

ADD ./symbolicatorprocessor ./symbolicatorprocessor
ADD ./builder-config.yaml ./
ADD ./Makefile ./
ADD ./config.yaml ./

RUN make build

FROM gcr.io/distroless/cc

COPY --from=build --chmod=755 /go/src/otelcol-dev/otelcol-dev /otelcol-dev
COPY --from=build  /go/src/config.yaml /
COPY --from=build /go/pkg/mod/github.com/honeycombio/ /go/pkg/mod/github.com/honeycombio/

ENTRYPOINT [ "/otelcol-dev" ]
CMD [ "--config", "config.yaml" ]
