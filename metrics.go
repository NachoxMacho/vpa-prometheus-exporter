package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
)

func StartMetricRecording(metricName string, metricFunc func() error, scrapeInterval time.Duration) {
	go func() {
		for {
			err := metricFunc()
			if err != nil {
				slog.Error("error when building metric",
					slog.String("metric", metricName),
					slog.String("error", err.Error()),
				)
			} else {
				slog.Info("successfully fetched metric",
					slog.String("metric", metricName),
				)
			}
			time.Sleep(scrapeInterval)
		}
	}()
}

func recordVPARecommendations(client *versioned.Clientset) func() error {
	type args struct {
		Opts       prometheus.GaugeOpts
		LabelNames []string
	}

	recommendationsOpts := args{
		Opts: prometheus.GaugeOpts{
			Name: "vpa_recommendations",
		},
		LabelNames: []string{
			"name",
			"namespace",
			"target_ref_name",
			"target_ref_kind",
			"container_name",
			"type",
			"resource",
			"unit",
		},
	}

	_ = prometheus.Register(prometheus.NewGaugeVec(recommendationsOpts.Opts, recommendationsOpts.LabelNames))

	return func() error {

		ctx, span := StartTrace(context.Background(), "")
		defer span.End()

		recommendations := prometheus.NewGaugeVec(recommendationsOpts.Opts, recommendationsOpts.LabelNames)

		vpas, err := client.AutoscalingV1().VerticalPodAutoscalers("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, v := range vpas.Items {
			promLabels := prometheus.Labels{
				"name":            v.Name,
				"namespace":       v.Namespace,
				"target_ref_name": v.Spec.TargetRef.Name,
				"target_ref_kind": v.Spec.TargetRef.Kind,
			}
			for _, cr := range v.Status.Recommendation.ContainerRecommendations {
				promLabels["container_name"] = cr.ContainerName
				tempGauge, err := recommendations.CurryWith(promLabels)
				if err != nil {
					slog.Error("failed to curry gauge", slog.String("error", err.Error()))
					continue
				}

				tempGauge.With(prometheus.Labels{
					"resource": "cpu",
					"unit":     "core",
					"type":     "target",
				}).Set(cr.Target.Cpu().AsApproximateFloat64())
				tempGauge.With(prometheus.Labels{
					"resource": "cpu",
					"unit":     "core",
					"type":     "lowerBound",
				}).Set(cr.LowerBound.Cpu().AsApproximateFloat64())
				tempGauge.With(prometheus.Labels{
					"resource": "cpu",
					"unit":     "core",
					"type":     "upperBound",
				}).Set(cr.UpperBound.Cpu().AsApproximateFloat64())
				tempGauge.With(prometheus.Labels{
					"resource": "cpu",
					"unit":     "core",
					"type":     "uncappedTarget",
				}).Set(cr.UncappedTarget.Cpu().AsApproximateFloat64())
				tempGauge.With(prometheus.Labels{
					"resource": "memory",
					"unit":     "byte",
					"type":     "target",
				}).Set(cr.Target.Memory().AsApproximateFloat64())
				tempGauge.With(prometheus.Labels{
					"resource": "memory",
					"unit":     "byte",
					"type":     "lowerBound",
				}).Set(cr.LowerBound.Memory().AsApproximateFloat64())
				tempGauge.With(prometheus.Labels{
					"resource": "memory",
					"unit":     "byte",
					"type":     "upperBound",
				}).Set(cr.UpperBound.Memory().AsApproximateFloat64())
				tempGauge.With(prometheus.Labels{
					"resource": "memory",
					"unit":     "byte",
					"type":     "uncappedTarget",
				}).Set(cr.UncappedTarget.Memory().AsApproximateFloat64())
			}
		}

		prometheus.Unregister(recommendations)
		_ = prometheus.Register(recommendations)
		return nil
	}
}
