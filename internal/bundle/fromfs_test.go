package bundle

import (
	"encoding/json"
	"testing"
	"testing/fstest"
)

func TestFromFS_Success(t *testing.T) {
	fsys := fstest.MapFS{
		"metadata/annotations.yaml": &fstest.MapFile{
			Data: []byte(`annotations:
  operators.operatorframework.io.bundle.package.v1: my-operator
  operators.operatorframework.io.bundle.channels.v1: stable
  operators.operatorframework.io.bundle.channel.default.v1: stable
`),
		},
		"manifests/csv.yaml": &fstest.MapFile{
			Data: []byte(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: my-operator.v1.0.0
spec:
  displayName: My Operator
  version: 1.0.0
  install:
    strategy: deployment
    spec:
      deployments: []
`),
		},
	}

	rv1, err := FromFS(fsys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rv1.PackageName != "my-operator" {
		t.Errorf("expected PackageName %q, got %q", "my-operator", rv1.PackageName)
	}
	if rv1.CSV.Name != "my-operator.v1.0.0" {
		t.Errorf("expected CSV name %q, got %q", "my-operator.v1.0.0", rv1.CSV.Name)
	}
}

func TestFromFS_CSVCRDAndOther(t *testing.T) {
	fsys := fstest.MapFS{
		"metadata/annotations.yaml": &fstest.MapFile{
			Data: []byte(`annotations:
  operators.operatorframework.io.bundle.package.v1: test-pkg
`),
		},
		"manifests/csv.yaml": &fstest.MapFile{
			Data: []byte(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: test-pkg.v1.0.0
spec:
  install:
    strategy: deployment
    spec:
      deployments: []
`),
		},
		"manifests/crd.yaml": &fstest.MapFile{
			Data: []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
spec:
  group: example.com
  names:
    kind: Widget
    plural: widgets
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
`),
		},
		"manifests/configmap.yaml": &fstest.MapFile{
			Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value
`),
		},
	}

	rv1, err := FromFS(fsys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rv1.PackageName != "test-pkg" {
		t.Errorf("expected PackageName %q, got %q", "test-pkg", rv1.PackageName)
	}
	if rv1.CSV.Name != "test-pkg.v1.0.0" {
		t.Errorf("expected CSV name %q, got %q", "test-pkg.v1.0.0", rv1.CSV.Name)
	}
	if len(rv1.CRDs) != 1 {
		t.Fatalf("expected 1 CRD, got %d", len(rv1.CRDs))
	}
	if rv1.CRDs[0].Name != "widgets.example.com" {
		t.Errorf("expected CRD name %q, got %q", "widgets.example.com", rv1.CRDs[0].Name)
	}
	if len(rv1.Others) != 1 {
		t.Fatalf("expected 1 Other, got %d", len(rv1.Others))
	}
	if rv1.Others[0].GetName() != "test-config" {
		t.Errorf("expected Other name %q, got %q", "test-config", rv1.Others[0].GetName())
	}
}

func TestFromFS_PropertiesMerge(t *testing.T) {
	fsys := fstest.MapFS{
		"metadata/annotations.yaml": &fstest.MapFile{
			Data: []byte(`annotations:
  operators.operatorframework.io.bundle.package.v1: props-pkg
`),
		},
		"metadata/properties.yaml": &fstest.MapFile{
			Data: []byte(`properties:
  - type: olm.package
    value: {"packageName": "props-pkg", "version": "1.0.0"}
`),
		},
		"manifests/csv.yaml": &fstest.MapFile{
			Data: []byte(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: props-pkg.v1.0.0
  annotations:
    olm.properties: '[{"type":"olm.gvk","value":{"group":"example.com","kind":"Widget","version":"v1"}}]'
spec:
  install:
    strategy: deployment
    spec:
      deployments: []
`),
		},
	}

	rv1, err := FromFS(fsys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	propsJSON, ok := rv1.CSV.Annotations["olm.properties"]
	if !ok {
		t.Fatal("expected olm.properties annotation")
	}

	var props []Property
	if err := json.Unmarshal([]byte(propsJSON), &props); err != nil {
		t.Fatalf("failed to unmarshal properties: %v", err)
	}
	if len(props) != 2 {
		t.Fatalf("expected 2 properties (1 existing + 1 from metadata), got %d", len(props))
	}
	if props[0].Type != "olm.gvk" {
		t.Errorf("expected first property type %q, got %q", "olm.gvk", props[0].Type)
	}
	if props[1].Type != "olm.package" {
		t.Errorf("expected second property type %q, got %q", "olm.package", props[1].Type)
	}
}

func TestFromFS_MissingAnnotations(t *testing.T) {
	fsys := fstest.MapFS{
		"manifests/csv.yaml": &fstest.MapFile{
			Data: []byte(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: test.v1.0.0
spec:
  install:
    strategy: deployment
    spec:
      deployments: []
`),
		},
	}

	_, err := FromFS(fsys)
	if err == nil {
		t.Fatal("expected error for missing annotations.yaml")
	}
}

func TestFromFS_MissingCSV(t *testing.T) {
	fsys := fstest.MapFS{
		"metadata/annotations.yaml": &fstest.MapFile{
			Data: []byte(`annotations:
  operators.operatorframework.io.bundle.package.v1: test-pkg
`),
		},
		"manifests/configmap.yaml": &fstest.MapFile{
			Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value
`),
		},
	}

	_, err := FromFS(fsys)
	if err == nil {
		t.Fatal("expected error for missing CSV")
	}
	expected := `no ClusterServiceVersion found in "manifests"`
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestFromFS_SubdirectoryInManifests(t *testing.T) {
	fsys := fstest.MapFS{
		"metadata/annotations.yaml": &fstest.MapFile{
			Data: []byte(`annotations:
  operators.operatorframework.io.bundle.package.v1: test-pkg
`),
		},
		"manifests/subdir/csv.yaml": &fstest.MapFile{
			Data: []byte(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: test.v1.0.0
spec:
  install:
    strategy: deployment
    spec:
      deployments: []
`),
		},
	}

	_, err := FromFS(fsys)
	if err == nil {
		t.Fatal("expected error for subdirectory in manifests")
	}
}
