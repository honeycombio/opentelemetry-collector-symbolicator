run: otelcol-dev
	go run ./otelcol-dev --config config.yaml

otelcol-dev: builder
	builder --config builder-config.yaml

builder:
	go install go.opentelemetry.io/collector/cmd/builder@latest

clean:
	rm -rf otelcol-dev
