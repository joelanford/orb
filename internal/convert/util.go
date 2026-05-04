package convert

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
)

const maxNameLength = 63

func ObjectNameForBaseAndSuffix(base string, suffix string) string {
	if len(base)+len(suffix) > maxNameLength {
		base = base[:maxNameLength-len(suffix)-1]
	}
	return fmt.Sprintf("%s-%s", base, suffix)
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

func DefaultUniqueNameGenerator(base string, o any) string {
	hashStr := DeepHashObject(o)
	return ObjectNameForBaseAndSuffix(base, hashStr)
}

func DeepHashObject(obj any) string {
	hasher := sha256.New224()
	encoder := json.NewEncoder(hasher)
	if err := encoder.Encode(obj); err != nil {
		panic(fmt.Sprintf("couldn't encode object: %v", err))
	}

	var i big.Int
	i.SetBytes(hasher.Sum(nil))
	return i.Text(36)
}
