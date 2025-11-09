package main

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func handleHealth() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, span := StartTrace(context.TODO(), "health", trace.WithAttributes(attribute.String("User-Agent", r.Header.Get("User-Agent"))))
			defer span.End()
			w.WriteHeader(204)
			span.SetStatus(codes.Ok, "completed call")
		},
	)
}
func handleReady() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, span := StartTrace(context.TODO(), "ready", trace.WithAttributes(attribute.String("User-Agent", r.Header.Get("User-Agent"))))
			defer span.End()
			w.WriteHeader(204)
			span.SetStatus(codes.Ok, "completed call")
		},
	)
}
