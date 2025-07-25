// Code generated by mdatagen. DO NOT EDIT.

package metadatatest

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processortest"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/metric/metricdata/metricdatatest"
)

func NewSettings(tt *componenttest.Telemetry) processor.Settings {
	set := processortest.NewNopSettings(processortest.NopType)
	set.ID = component.NewID(component.MustNewType("source_map_symbolicator"))
	set.TelemetrySettings = tt.NewTelemetrySettings()
	return set
}

func AssertEqualProcessorSourceMapCacheSize(t *testing.T, tt *componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_processor_source_map_cache_size",
		Description: "Size of the source map cache in bytes.",
		Unit:        "{sourcemaps}",
		Data: metricdata.Gauge[int64]{
			DataPoints: dps,
		},
	}
	got, err := tt.GetMetric("otelcol_processor_source_map_cache_size")
	require.NoError(t, err)
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualProcessorSymbolicationDuration(t *testing.T, tt *componenttest.Telemetry, dps []metricdata.HistogramDataPoint[float64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_processor_symbolication_duration",
		Description: "Duration in seconds taken to symbolicate frames.",
		Unit:        "s",
		Data: metricdata.Histogram[float64]{
			Temporality: metricdata.CumulativeTemporality,
			DataPoints:  dps,
		},
	}
	got, err := tt.GetMetric("otelcol_processor_symbolication_duration")
	require.NoError(t, err)
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualProcessorTotalFailedFrames(t *testing.T, tt *componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_processor_total_failed_frames",
		Description: "Total number of frames the symbolicator failed to symbolicate.",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got, err := tt.GetMetric("otelcol_processor_total_failed_frames")
	require.NoError(t, err)
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualProcessorTotalProcessedFrames(t *testing.T, tt *componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_processor_total_processed_frames",
		Description: "Total number of frames the symbolicator processed.",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got, err := tt.GetMetric("otelcol_processor_total_processed_frames")
	require.NoError(t, err)
	metricdatatest.AssertEqual(t, want, got, opts...)
}

func AssertEqualProcessorTotalSourceMapFetchFailures(t *testing.T, tt *componenttest.Telemetry, dps []metricdata.DataPoint[int64], opts ...metricdatatest.Option) {
	want := metricdata.Metrics{
		Name:        "otelcol_processor_total_source_map_fetch_failures",
		Description: "Total number of source map fetch failures.",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
			DataPoints:  dps,
		},
	}
	got, err := tt.GetMetric("otelcol_processor_total_source_map_fetch_failures")
	require.NoError(t, err)
	metricdatatest.AssertEqual(t, want, got, opts...)
}
