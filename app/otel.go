package app

import (
	"context"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

func InitOtel() {
	spanExp, err := autoexport.NewSpanExporter(context.TODO())
	if err != nil {
		panic(err)
	}
	otel.SetTracerProvider(trace.NewTracerProvider(trace.WithBatcher(spanExp)))

	metricReader, err := autoexport.NewMetricReader(context.TODO())
	if err != nil {
		panic(err)
	}
	otel.SetMeterProvider(metric.NewMeterProvider(metric.WithReader(metricReader)))

	logExp, err := autoexport.NewLogExporter(context.TODO())
	if err != nil {
		panic(err)
	}
	global.SetLoggerProvider(log.NewLoggerProvider(log.WithProcessor(log.NewBatchProcessor(logExp))))
}
