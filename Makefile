.PHONY: builder
builder:
	go install go.opentelemetry.io/collector/cmd/builder@v0.127.0

.PHONY: clean
clean:
	rm -rf otelcol-dev

.PHONY: build
build: builder
	builder --config builder-config.yaml

.PHONY: build-docker
build-docker:
	docker buildx build . -t collector-symbolicator-processor

.PHONY: run
run: build
	go run ./otelcol-dev --config config.yaml

.PHONY: test
test: build
	go test ./symbolicatorprocessor/ ./dsymprocessor
