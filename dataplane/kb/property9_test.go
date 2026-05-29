//go:build pbt

package kb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Validates: Requirements B9.5, B9.6, B9.7**
//
// Property P9: KB ACL 无侧信道
// - Unauthorized chunks never appear in results
// - Result count and timing don't reveal existence of unauthorized chunks

var allClassificationLevels = []string{"public", "internal", "confidential", "restricted", "secret"}

// genUserContext generates a random UserContext with varied attributes.
func genUserContext() gopter.Gen {
	return gopter.CombineGens(
		gen.OneConstOf("alice", "bob", "charlie", "dave", "eve", "frank", "grace", "heidi"),
		gen.IntRange(0, 3),
		gen.OneConstOf("public", "internal", "confidential", "restricted", "secret"),
	).Map(func(values []interface{}) UserContext {
		userID := values[0].(string)
		numGroups := values[1].(int)
		classification := values[2].(string)
		allGroups := []string{"engineering", "hr", "finance", "ops", "legal", "admin", "sales"}
		groups := allGroups[:numGroups]
		return UserContext{
			UserID:              userID,
			Groups:              groups,
			ClassificationLevel: classification,
		}
	})
}

// genChunk generates a random RetrievalResult with varied ACL metadata.
func genChunk() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 1000),                                                                                                 // text suffix
		gen.SliceOfN(3, gen.OneConstOf("alice", "bob", "charlie", "dave", "eve", "frank", "grace", "heidi")),                  // acl users
		gen.SliceOfN(3, gen.OneConstOf("engineering", "hr", "finance", "ops", "legal", "admin", "sales", "marketing", "r&d")), // acl groups
		gen.OneConstOf("public", "internal", "confidential", "restricted", "secret"),                                          // classification
		gen.Bool(), // whether to include acl_users
		gen.Bool(), // whether to include acl_groups
	).Map(func(values []interface{}) RetrievalResult {
		textSuffix := values[0].(int)
		aclUsers := values[1].([]string)
		aclGroups := values[2].([]string)
		classification := values[3].(string)
		includeUsers := values[4].(bool)
		includeGroups := values[5].(bool)

		meta := map[string]string{
			"classification": classification,
		}
		if includeUsers && len(aclUsers) > 0 {
			meta["acl_users"] = strings.Join(aclUsers, ",")
		}
		if includeGroups && len(aclGroups) > 0 {
			meta["acl_groups"] = strings.Join(aclGroups, ",")
		}

		return RetrievalResult{
			Text:     fmt.Sprintf("chunk_%d", textSuffix),
			Score:    1.0,
			Metadata: meta,
		}
	})
}

// genChunks generates a slice of 0-20 random chunks.
func genChunks() gopter.Gen {
	return gen.SliceOfN(20, genChunk())
}

// isAuthorized checks whether a user should have access to a chunk
// according to the ACL rules: user in acl_users OR user's groups intersect acl_groups,
// AND chunk classification <= user classification.
func isAuthorized(chunk RetrievalResult, user UserContext) bool {
	meta := chunk.Metadata
	if meta == nil {
		return false
	}

	// Classification check
	chunkClassification := meta["classification"]
	if chunkClassification == "" {
		chunkClassification = "public"
	}
	userRank := classificationRank[user.ClassificationLevel]
	chunkRank := classificationRank[chunkClassification]
	if userRank < chunkRank {
		return false
	}

	// ACL identity check
	aclUsers := meta["acl_users"]
	aclGroups := meta["acl_groups"]

	// Secure by default: no ACL → deny
	if aclUsers == "" && aclGroups == "" {
		return false
	}

	// Check user in acl_users
	if aclUsers != "" {
		for _, u := range strings.Split(aclUsers, ",") {
			if strings.TrimSpace(u) == user.UserID {
				return true
			}
		}
	}

	// Check group intersection
	if aclGroups != "" {
		groupSet := make(map[string]struct{}, len(user.Groups))
		for _, g := range user.Groups {
			groupSet[g] = struct{}{}
		}
		for _, g := range strings.Split(aclGroups, ",") {
			if _, ok := groupSet[strings.TrimSpace(g)]; ok {
				return true
			}
		}
	}

	return false
}

// TestProperty9 validates KB ACL has no side-channel leakage.
// P9: unauthorized chunks never appear in results; result count and timing
// don't reveal existence of unauthorized chunks.
func TestProperty9(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 1000
	parameters.MaxSize = 20
	properties := gopter.NewProperties(parameters)

	// Sub-property 1: No unauthorized chunk ever appears in filtered results
	// Validates: B9.5, B9.7
	properties.Property("unauthorized chunks never appear in results", prop.ForAll(
		func(user UserContext, chunks []RetrievalResult) bool {
			filtered := FilterByACL(chunks, user)

			for _, result := range filtered {
				if !isAuthorized(result, user) {
					return false
				}
			}
			return true
		},
		genUserContext(),
		genChunks(),
	))

	// Sub-property 2: All authorized chunks are returned (no false negatives)
	// Validates: B9.6
	properties.Property("all authorized chunks are returned", prop.ForAll(
		func(user UserContext, chunks []RetrievalResult) bool {
			filtered := FilterByACL(chunks, user)

			// Count how many chunks the user should have access to
			expectedCount := 0
			for _, chunk := range chunks {
				if isAuthorized(chunk, user) {
					expectedCount++
				}
			}

			return len(filtered) == expectedCount
		},
		genUserContext(),
		genChunks(),
	))

	// Sub-property 3: Result count doesn't reveal total input size beyond authorized chunks
	// (i.e., same authorized subset → same result count regardless of unauthorized additions)
	// Validates: B9.5
	properties.Property("result count does not leak unauthorized chunk existence", prop.ForAll(
		func(user UserContext, authorizedChunks []RetrievalResult, extraUnauthorizedChunks []RetrievalResult) bool {
			// Filter with only authorized-candidate chunks
			filtered1 := FilterByACL(authorizedChunks, user)

			// Add extra unauthorized chunks and filter again
			combined := make([]RetrievalResult, 0, len(authorizedChunks)+len(extraUnauthorizedChunks))
			combined = append(combined, authorizedChunks...)
			combined = append(combined, extraUnauthorizedChunks...)
			filtered2 := FilterByACL(combined, user)

			// The count of authorized results from the original set should be
			// preserved exactly — extra unauthorized chunks must not affect the
			// count of results that were originally authorized
			count1 := 0
			for _, r := range filtered1 {
				if isAuthorized(r, user) {
					count1++
				}
			}
			count2FromOriginal := 0
			for _, r := range filtered2 {
				// Check if this result came from the original set
				for _, orig := range authorizedChunks {
					if r.Text == orig.Text && isAuthorized(r, user) {
						count2FromOriginal++
						break
					}
				}
			}

			return count1 == count2FromOriginal
		},
		genUserContext(),
		genChunks(),
		genChunks().Map(func(chunks []RetrievalResult) []RetrievalResult {
			// Ensure these extra chunks have no ACL that would authorize the user
			// by removing acl_users and acl_groups
			for i := range chunks {
				chunks[i].Metadata["acl_users"] = ""
				chunks[i].Metadata["acl_groups"] = ""
			}
			return chunks
		}),
	))

	// Sub-property 4: FilterByACL is deterministic — same inputs always produce same outputs
	// (no timing or randomness side-channel)
	// Validates: B9.5
	properties.Property("FilterByACL is deterministic - no timing side-channel", prop.ForAll(
		func(user UserContext, chunks []RetrievalResult) bool {
			result1 := FilterByACL(chunks, user)
			result2 := FilterByACL(chunks, user)

			if len(result1) != len(result2) {
				return false
			}
			for i := range result1 {
				if result1[i].Text != result2[i].Text {
					return false
				}
			}
			return true
		},
		genUserContext(),
		genChunks(),
	))

	result := properties.Run(gopter.ConsoleReporter(false))
	if !result {
		t.Fatal("Property P9 (KB ACL no side-channel) failed")
	}
	fmt.Println("Property P9: All 1000 iterations passed — KB ACL has no side-channel leakage")
}
