package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRegv1Dir_Read(t *testing.T) {
	// Create a temp directory with a valid regv1 bundle layout
	tmpDir := t.TempDir()

	// Create metadata directory
	metadataDir := filepath.Join(tmpDir, "metadata")
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		t.Fatalf("failed to create metadata dir: %v", err)
	}

	// Create manifests directory
	manifestsDir := filepath.Join(tmpDir, "manifests")
	if err := os.MkdirAll(manifestsDir, 0755); err != nil {
		t.Fatalf("failed to create manifests dir: %v", err)
	}

	// Write annotations.yaml
	annotationsYAML := []byte(`annotations:
  operators.operatorframework.io.bundle.package.v1: test-operator
  operators.operatorframework.io.bundle.channels.v1: alpha
`)
	if err := os.WriteFile(filepath.Join(metadataDir, "annotations.yaml"), annotationsYAML, 0644); err != nil {
		t.Fatalf("failed to write annotations.yaml: %v", err)
	}

	// Write a minimal CSV
	csvYAML := []byte(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: test-operator.v0.1.0
spec:
  displayName: Test Operator
  version: 0.1.0
  install:
    strategy: deployment
    spec:
      deployments: []
`)
	if err := os.WriteFile(filepath.Join(manifestsDir, "csv.yaml"), csvYAML, 0644); err != nil {
		t.Fatalf("failed to write csv.yaml: %v", err)
	}

	src := &regv1Dir{ref: tmpDir}
	rv1, err := src.Read(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rv1.PackageName != "test-operator" {
		t.Errorf("expected PackageName %q, got %q", "test-operator", rv1.PackageName)
	}
	if rv1.CSV.Name != "test-operator.v0.1.0" {
		t.Errorf("expected CSV name %q, got %q", "test-operator.v0.1.0", rv1.CSV.Name)
	}
}

func TestRegv1Dir_Read_InvalidDir(t *testing.T) {
	src := &regv1Dir{ref: "/nonexistent/path"}
	_, err := src.Read(context.Background())
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}
