FROM golang:1.23 AS build

WORKDIR /go/src

RUN mkdir -p -m 0700 ~/.ssh && ssh-keyscan github.com >> ~/.ssh/known_hosts
RUN git config --global url."git@github.com:".insteadOf "https://github.com/"

ADD ./ ./
RUN make clean
ARG GOPRIVATE=github.com/honeycombio/symbolic-go
RUN --mount=type=ssh make build

ENTRYPOINT ["/otelcol-dev"]
CMD ["--config", "config.yaml"]