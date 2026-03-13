package integration_test

import (
	"context"
	"testing"
	"time"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/safety"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func makeIntegrationOOMPod(podName, namespace, containerName string, restartCount int32) *corev1.Pod {
	oomTime := metav1.NewTime(time.Now().Add(-30 * time.Minute))
	memLimit := resource.NewQuantity(256*1024*1024, resource.BinarySI)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "my-deploy-7d9f8b",
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: containerName,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: *memLimit,
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         containerName,
					RestartCount: restartCount,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode:   137,
							Reason:     "OOMKilled",
							FinishedAt: oomTime,
						},
					},
				},
			},
		},
	}
}

type oomDetectorAdapter struct {
	detector *safety.OOMDetector
}

func (a *oomDetectorAdapter) GetMemoryBoostFactor(ns, workload, container string) float64 {
	return a.detector.GetMemoryBoostFactor(ns, workload, container)
}

func (a *oomDetectorAdapter) GetOOMHistory(ns, workload string) *recommendation.OOMHistoryInfo {
	h := a.detector.GetOOMHistory(ns, workload)
	if h == nil {
		return nil
	}
	info := &recommendation.OOMHistoryInfo{
		HasOOMHistory: h.TotalOOMs > 0,
		TotalOOMCount: h.TotalOOMs,
		ContainerOOMs: make(map[string]recommendation.ContainerOOMDetails),
	}
	for name, c := range h.ContainerOOMs {
		info.ContainerOOMs[name] = recommendation.ContainerOOMDetails{
			OOMCount:         c.OOMCount,
			RecommendedBoost: c.RecommendedBoost,
		}
	}
	return info
}

func TestSafetyPipeline_OOMDetectorBoostsRecommendation(t *testing.T) {
	pod := makeIntegrationOOMPod("my-deploy-7d9f8b-pod1", "default", "app", 3)
	kubeClient := fake.NewSimpleClientset(pod)
	detector := safety.NewOOMDetector(kubeClient)

	_, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}

	boost := detector.GetMemoryBoostFactor("default", "my-deploy", "app")
	if boost <= 1.0 {
		t.Errorf("expected boost > 1.0 for OOM-affected container, got %.2f", boost)
	}

	st := populatedStorage(200, 200, 200, 128*1024*1024, 128*1024*1024, 24*time.Hour)
	provider := &storageProvider{st}
	eng := recommendation.NewEngine()
	cfg := &v1alpha1.OptimizerConfig{
		Spec: v1alpha1.OptimizerConfigSpec{
			Enabled:          true,
			TargetNamespaces: []string{"default"},
			Strategy:         v1alpha1.StrategyBalanced,
			Recommendations: &v1alpha1.RecommendationConfig{
				CPUPercentile:   95,
				MinSamples:      10,
				SafetyMargin:    1.0,
				HistoryDuration: "24h",
			},
		},
	}

	adapter := &oomDetectorAdapter{detector: detector}
	recsWithOOM, err := eng.GenerateRecommendationsWithOOM(provider, adapter, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendationsWithOOM error: %v", err)
	}
	recsWithout, err := eng.GenerateRecommendations(provider, cfg)
	if err != nil {
		t.Fatalf("GenerateRecommendations error: %v", err)
	}

	if len(recsWithOOM) == 0 || len(recsWithout) == 0 {
		t.Skip("no recommendations generated")
	}

	memWithOOM := recsWithOOM[0].Containers[0].RecommendedMemory
	memWithout := recsWithout[0].Containers[0].RecommendedMemory
	if memWithOOM < memWithout {
		t.Errorf("OOM boost should increase memory: withOOM=%d, without=%d", memWithOOM, memWithout)
	}
}

func TestSafetyPipeline_OOMHistoryRetrieval(t *testing.T) {
	pod := makeIntegrationOOMPod("worker-abc123-pod1", "production", "worker", 2)
	kubeClient := fake.NewSimpleClientset(pod)
	detector := safety.NewOOMDetector(kubeClient)

	results, err := detector.CheckNamespaceForOOMs(context.Background(), "production")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one OOM result")
	}

	all := detector.GetAllOOMWorkloads()
	if len(all) == 0 {
		t.Error("expected non-empty OOM workload history")
	}
}

func TestSafetyPipeline_OOMClearHistory(t *testing.T) {
	pod := makeIntegrationOOMPod("cache-xyz-pod1", "default", "cache", 1)
	kubeClient := fake.NewSimpleClientset(pod)
	detector := safety.NewOOMDetector(kubeClient)

	_, err := detector.CheckNamespaceForOOMs(context.Background(), "default")
	if err != nil {
		t.Fatalf("CheckNamespaceForOOMs error: %v", err)
	}

	detector.ClearOOMHistory(0)
	all := detector.GetAllOOMWorkloads()
	if len(all) != 0 {
		t.Errorf("expected all OOM history cleared, got %d entries", len(all))
	}
}
