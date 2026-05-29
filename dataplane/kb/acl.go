// Package kb provides knowledge base data plane components including ACL filtering.
package kb

import "strings"

// ClassificationLevel represents the security classification of a chunk.
// Hierarchy: public(0) < internal(1) < confidential(2) < restricted(3) < secret(4).
var classificationRank = map[string]int{
	"public":       0,
	"internal":     1,
	"confidential": 2,
	"restricted":   3,
	"secret":       4,
}

// UserContext holds the requesting user's identity and access attributes.
type UserContext struct {
	UserID              string
	Groups              []string
	TenantID            string
	ClassificationLevel string
}

// RetrievalResult represents a single retrieval result from the knowledge base.
type RetrievalResult struct {
	Text     string
	Score    float64
	Metadata map[string]string
	SourceID string
}

// FilterByACL filters retrieval results based on the user's ACL permissions.
// Chunks without permission are removed (pre_filter enforcement mode).
// Secure by default: chunks with no ACL metadata are denied.
func FilterByACL(results []RetrievalResult, userContext UserContext) []RetrievalResult {
	filtered := make([]RetrievalResult, 0, len(results))
	for _, r := range results {
		if checkAccess(r, userContext) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func checkAccess(result RetrievalResult, ctx UserContext) bool {
	meta := result.Metadata
	if meta == nil {
		return false
	}

	// Classification check
	chunkClassification := meta["classification"]
	if chunkClassification == "" {
		chunkClassification = "public"
	}
	if !checkClassification(ctx.ClassificationLevel, chunkClassification) {
		return false
	}

	// ACL identity check
	aclUsers := meta["acl_users"]
	aclGroups := meta["acl_groups"]

	// Secure by default: no ACL → deny
	if aclUsers == "" && aclGroups == "" {
		return false
	}

	// Check user ID
	if aclUsers != "" {
		for _, u := range strings.Split(aclUsers, ",") {
			if strings.TrimSpace(u) == ctx.UserID {
				return true
			}
		}
	}

	// Check group membership
	if aclGroups != "" {
		groupSet := make(map[string]struct{}, len(ctx.Groups))
		for _, g := range ctx.Groups {
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

func checkClassification(userLevel, chunkLevel string) bool {
	userRank := classificationRank[userLevel]
	chunkRank := classificationRank[chunkLevel]
	return userRank >= chunkRank
}
