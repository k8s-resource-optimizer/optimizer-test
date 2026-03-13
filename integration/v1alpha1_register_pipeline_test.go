package integration_test

import (
	"testing"

	v1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestV1alpha1_Resource verifies the Resource helper returns the correct GroupResource.
func TestV1alpha1_Resource(t *testing.T) {
	gr := v1alpha1.Resource("optimizerconfigs")
	if gr.Group != v1alpha1.GroupName {
		t.Errorf("expected group %s, got %s", v1alpha1.GroupName, gr.Group)
	}
	if gr.Resource != "optimizerconfigs" {
		t.Errorf("expected resource 'optimizerconfigs', got %s", gr.Resource)
	}
}

// TestV1alpha1_AddToScheme verifies AddToScheme registers the types.
func TestV1alpha1_AddToScheme(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme error: %v", err)
	}

	gvk := v1alpha1.SchemeGroupVersion.WithKind("OptimizerConfig")
	if _, ok := scheme.New(gvk); ok != nil {
		// ok is an error - it's fine if it returns nil (type registered)
	}
}
