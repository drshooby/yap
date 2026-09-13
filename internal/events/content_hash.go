package events

import (
	"crypto/sha256"
	"encoding/hex"
)

// Text hashing for analysis-side use only.
//
// No event in the schema carries a hash of its text — see the "No hashes in the
// schema" section of docs/design.md for why. This helper exists because analysis
// needs to key caches by text: the embedding cache in `analyze entropy` avoids
// re-embedding a belief it has already paid for, and a judge-model pass would
// deduplicate candidate texts the same way.
//
// It hashes exactly the string it is given. Normalization, if any is wanted,
// belongs where model responses enter the system, so that the text stored in the
// log and the text that was hashed are always the same bytes.
func ContentHash(in string) string {
	sha := sha256.Sum256([]byte(in))
	return hex.EncodeToString(sha[:])
}
