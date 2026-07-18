package jj

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCollectSmoke runs Collect against a real jj repo given via
// JJ_SMOKE_REPO, printing the result for manual comparison.
func TestCollectSmoke(t *testing.T) {
	root := os.Getenv("JJ_SMOKE_REPO")
	if root == "" {
		t.Skip("JJ_SMOKE_REPO not set")
	}
	r := require.New(t)
	info, err := Collect(root, 8, 10)
	r.NoError(err)
	out, err := json.MarshalIndent(info, "", "  ")
	r.NoError(err)
	t.Logf("collected:\n%s", out)
}
