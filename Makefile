run: otelcol-dev
	go run ./otelcol-dev --config config.yaml

otelcol-dev: builder
	builder --config builder-config.yaml

builder:
	go install go.opentelemetry.io/collector/cmd/builder@v0.111.0

clean:
	rm -rf otelcol-dev
