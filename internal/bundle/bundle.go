package bundle

// Bundle is the intermediate representation that sources produce and destinations consume.
type Bundle struct {
	Name      string
	Version   string
	Manifests []Manifest
	Metadata  map[string]string
}

// Manifest represents a single Kubernetes manifest within a bundle.
type Manifest struct {
	Filename string
	Content  []byte
}
