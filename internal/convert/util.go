package convert

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const maxNameLength = 63

func ObjectNameForBaseAndSuffix(base string, suffix string) string {
	if len(base)+len(suffix) > maxNameLength {
		base = base[:maxNameLength-len(suffix)-1]
	}
	return fmt.Sprintf("%s-%s", base, suffix)
}

// ToUnstructured converts obj into an Unstructured. It expects the obj's gvk to be defined. If it is not,
// an error will be returned.
func ToUnstructured(obj client.Object) (*unstructured.Unstructured, error) {
	if obj == nil {
		return nil, errors.New("object is nil")
	}

	gvk := obj.GetObjectKind().GroupVersionKind()
	if len(gvk.Kind) == 0 {
		return nil, errors.New("object has no kind")
	}
	if len(gvk.Version) == 0 {
		return nil, errors.New("object has no version")
	}

	var u unstructured.Unstructured
	uObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("convert %s %q to unstructured: %w", gvk.Kind, obj.GetName(), err)
	}
	unstructured.RemoveNestedField(uObj, "metadata", "creationTimestamp")
	u.Object = uObj
	u.SetGroupVersionKind(gvk)
	return &u, nil
}

func MergeMaps(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// DeepHashObject writes specified object to hash based on the object's
// JSON representation.
func DeepHashObject(obj interface{}) string {
	hasher := sha256.New224()
	encoder := json.NewEncoder(hasher)
	if err := encoder.Encode(obj); err != nil {
		panic(fmt.Sprintf("couldn't encode object: %v", err))
	}

	var i big.Int
	i.SetBytes(hasher.Sum(nil))
	return i.Text(36)
}
