FROM golang:1.23 AS build

WORKDIR /go/src

RUN mkdir -p -m 0700 ~/.ssh && ssh-keyscan github.com >> ~/.ssh/known_hosts
RUN git config --global url."git@github.com:".insteadOf "https://github.com/"

ADD ./processor ./processor
ADD ./builder-config.yaml ./
ADD ./Makefile ./
ADD ./config.yaml ./

ARG GOPRIVATE=github.com/honeycombio/symbolic-go
RUN --mount=type=ssh make build

FROM gcr.io/distroless/cc

COPY --from=build --chmod=755 /go/src/otelcol-dev/otelcol-dev /otelcol-dev
COPY --from=build  /go/src/config.yaml /
COPY --from=build /go/pkg/mod/github.com/honeycombio/symbolic-go@v0.0.2/lib/linux_aarch64/libsymbolic_cabi.so /go/pkg/mod/github.com/honeycombio/symbolic-go@v0.0.2/lib/linux_aarch64/

ENTRYPOINT [ "/otelcol-dev" ]
CMD [ "--config", "config.yaml" ]