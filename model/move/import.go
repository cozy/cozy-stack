package move

// ImportOptions contains the options for launching the import worker.
// TODO document it in docs/workers.md
type ImportOptions struct {
	ManifestURL string `json:"manifest_url"`
}
