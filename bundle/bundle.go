package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// ManifestHash computes the identity of a complete configuration set: the
// SHA-256 over the sorted "id:sha256" lines of its files. Both sides use
// this one implementation — the cloud when a bundle is created, the
// Supervisor when a delivered bundle is verified — so they can never
// disagree about what a complete revision is.
func ManifestHash(fileSHAs map[string]string) string {
	ids := make([]string, 0, len(fileSHAs))
	for id := range fileSHAs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	h := sha256.New()
	for _, id := range ids {
		// hash.Hash writes never fail; errcheck wants that made explicit.
		_, _ = fmt.Fprintf(h, "%s:%s\n", id, fileSHAs[id])
	}
	return hex.EncodeToString(h.Sum(nil))
}
