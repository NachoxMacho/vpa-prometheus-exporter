package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/fields"
	autoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	kubernetesClientset "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	"k8s.io/client-go/tools/cache"
)

func setupVPAWatcher(client *kubernetesClientset.Clientset) error {
	type args struct {
		Opts       prometheus.GaugeOpts
		LabelNames []string
	}

	recommendationsOptions := prometheus.GaugeOpts{
		Name: "vpa_recommendations",
		Help: "The recommendations calculated by the vpa resource",
	}
	recommendationsLabels := []string{
		"name",
		"namespace",
		"target_ref_name",
		"target_ref_kind",
		"container_name",
		"type",
		"resource",
		"unit",
	}

	recommendations := prometheus.NewGaugeVec(recommendationsOptions, recommendationsLabels)

	_, c := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: cache.NewListWatchFromClient(client.AutoscalingV1().RESTClient(), "verticalpodautoscalers", "", fields.Everything()),
		ResyncPeriod:  time.Minute,
		Handler: cache.ResourceEventHandlerFuncs{
			DeleteFunc: func(obj any) {
				v := obj.(*autoscalingv1.VerticalPodAutoscaler)
				recommendations.DeletePartialMatch(prometheus.Labels{
					"name":            v.Name,
					"namespace":       v.Namespace,
					"target_ref_name": v.Spec.TargetRef.Name,
					"target_ref_kind": v.Spec.TargetRef.Kind,
				})
			},
			AddFunc: func(obj any) {
				v := obj.(*autoscalingv1.VerticalPodAutoscaler)
				recordRecommendations(v, recommendations)
			},
			UpdateFunc: func(oldObj, newObj any) {
				v := newObj.(*autoscalingv1.VerticalPodAutoscaler)
				recordRecommendations(v, recommendations)
			},
		},
		ObjectType: &autoscalingv1.VerticalPodAutoscaler{},
	})

	go func() {
		c.Run(context.Background().Done())
	}()

	return prometheus.Register(recommendations)
}

func recordRecommendations(v *autoscalingv1.VerticalPodAutoscaler, recommendations *prometheus.GaugeVec) {
	promLabels := prometheus.Labels{
		"name":            v.Name,
		"namespace":       v.Namespace,
		"target_ref_name": v.Spec.TargetRef.Name,
		"target_ref_kind": v.Spec.TargetRef.Kind,
	}
	if nil == v.Status.Recommendation {
		return
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
