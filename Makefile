PROCESSOR_DIRS := sourcemapprocessor dsymprocessor proguardprocessor
MDATAGEN := $(CURDIR)/bin/mdatagen

.PHONY: builder
builder:
	go install go.opentelemetry.io/collector/cmd/builder@v0.142.0

.PHONY: clean
clean:
	rm -rf otelcol-dev
	rm -rf bin/

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
	go test ./sourcemapprocessor/ ./dsymprocessor ./proguardprocessor

.PHONY: generate-docs
generate-docs: $(MDATAGEN) build
	@for dir in $(PROCESSOR_DIRS); do \
		echo "Running mdatagen in $$dir..."; \
		(cd $$dir && $(MDATAGEN) metadata.yaml) || exit 1; \
	done

.PHONY: $(MDATAGEN)
$(MDATAGEN):
	@required=$$(grep 'go.opentelemetry.io/collector/' builder-config.yaml | head -1 | grep --only-matching --extended-regexp 'v[0-9]+\.[0-9]+\.[0-9]+'); \
	if [ -x "$(MDATAGEN)" ]; then \
		installed=$$($(MDATAGEN) --version 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1); \
		if [ "$$installed" = "$$required" ]; then \
			echo "✅ mdatagen $$required is installed"; \
			exit 0; \
		fi; \
		echo "⚠️ mdatagen version mismatch: installed $$installed, need $$required"; \
	fi; \
	echo "🧰 Installing mdatagen $$required to $(MDATAGEN) (temporarily cloning opentelemetry-collector at $$required)..."; \
	tmpdir=$$(mktemp -d); \
	git -c advice.detachedHead=false clone --depth=1 --branch $$required git@github.com:open-telemetry/opentelemetry-collector.git "$$tmpdir/otelcol" || exit 1; \
	(cd "$$tmpdir/otelcol/cmd/mdatagen" && GOBIN=$(CURDIR)/bin go install .) || exit 1; \
	echo "🧹 Removing temporary clone of opentelemetry-collector at $$required"; \
	rm -rf "$$tmpdir"
