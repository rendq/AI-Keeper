//go:build pbt

// Feature: ai-platform, Property 32: Hold 阻断 GC 不变量
//
// Generator: Random (Hold set × GCCandidate batch)
// Oracle: active hold 覆盖范围内的对象 ⇒ ShouldDelete==false;
//         无 active hold 且 retention 已过期 ⇒ ShouldDelete==true
// Property: P32 / Validates: D4.2

package holds

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

func genTenant() gopter.Gen {
	return gen.OneConstOf("tenant-a", "tenant-b", "tenant-c", "tenant-d")
}

func genHoldStatus() gopter.Gen {
	return gen.OneConstOf(StatusActive, StatusPendingRelease, StatusReleased)
}

// holdInput is the intermediate struct used by the generator.
type holdInput struct {
	Tenant    string
	Status    HoldStatus
	HasRange  bool
	StartDay  int // offset days from baseTime
	RangeDays int // duration of the time range in days
}

func genHoldInput() gopter.Gen {
	return gen.Struct(reflect.TypeOf(holdInput{}), map[string]gopter.Gen{
		"Tenant":    genTenant(),
		"Status":    genHoldStatus(),
		"HasRange":  gen.Bool(),
		"StartDay":  gen.IntRange(-30, 60),
		"RangeDays": gen.IntRange(1, 90),
	})
}

type gcCandidateInput struct {
	Tenant    string
	DayOffset int // offset days from baseTime for CreatedAt
}

func genGCCandidateInput() gopter.Gen {
	return gen.Struct(reflect.TypeOf(gcCandidateInput{}), map[string]gopter.Gen{
		"Tenant":    genTenant(),
		"DayOffset": gen.IntRange(-30, 120),
	})
}

type property32Input struct {
	Holds      []holdInput
	Candidates []gcCandidateInput
}

func genHoldSlice() gopter.Gen {
	return gen.IntRange(0, 8).FlatMap(func(n interface{}) gopter.Gen {
		return gen.SliceOfN(n.(int), genHoldInput())
	}, reflect.TypeOf([]holdInput{}))
}

func genCandidateSlice() gopter.Gen {
	return gen.IntRange(1, 10).FlatMap(func(n interface{}) gopter.Gen {
		return gen.SliceOfN(n.(int), genGCCandidateInput())
	}, reflect.TypeOf([]gcCandidateInput{}))
}

func genProperty32Input() gopter.Gen {
	return gen.Struct(reflect.TypeOf(property32Input{}), map[string]gopter.Gen{
		"Holds":      genHoldSlice(),
		"Candidates": genCandidateSlice(),
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var baseTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// isCoveredByActiveHold mirrors the logic that determines if a candidate is
// covered by any active hold (same tenant, time range overlap if specified).
func isCoveredByActiveHold(candidate GCCandidate, holds []*Hold) bool {
	for _, h := range holds {
		if h.Status != StatusActive {
			continue
		}
		// Check tenant match.
		tenantMatch := false
		for _, t := range h.Scope.Tenants {
			if t == candidate.Tenant {
				tenantMatch = true
				break
			}
		}
		if !tenantMatch {
			continue
		}
		// Check time range if specified.
		hasRange := !h.Scope.TimeRange.Start.IsZero() || !h.Scope.TimeRange.End.IsZero()
		if hasRange {
			if candidate.CreatedAt.Before(h.Scope.TimeRange.Start) || candidate.CreatedAt.After(h.Scope.TimeRange.End) {
				continue
			}
		}
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// TestProperty32
// ---------------------------------------------------------------------------

// **Validates: Requirements D4.2**
func TestProperty32(t *testing.T) {
	seed := time.Now().UnixNano()
	if s, ok := os.LookupEnv("AIP_PBT_SEED"); ok {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = v
		}
	}
	t.Logf("PBT seed: %d (set AIP_PBT_SEED to reproduce)", seed)

	params := gopter.DefaultTestParametersWithSeed(seed)
	params.MinSuccessfulTests = 1000
	params.MaxShrinkCount = 200

	properties := gopter.NewProperties(params)

	// Use a retention policy with 30 days — candidates older than 30 days are
	// eligible for deletion (unless held).
	retentionDays := 30

	properties.Property("active hold blocks GC deletion", prop.ForAll(
		func(input property32Input) (bool, error) {
			// Build hold store.
			store := NewMemoryStore()
			var allHolds []*Hold

			for i, hi := range input.Holds {
				h := &Hold{
					ID:        fmt.Sprintf("hold-%d", i),
					Reason:    "compliance",
					AppliedBy: "system",
					AppliedAt: baseTime,
					Status:    hi.Status,
					Scope: HoldScope{
						Tenants: []string{hi.Tenant},
					},
				}
				if hi.HasRange {
					h.Scope.TimeRange = TimeRange{
						Start: baseTime.AddDate(0, 0, hi.StartDay),
						End:   baseTime.AddDate(0, 0, hi.StartDay+hi.RangeDays),
					}
				}
				if err := store.Create(h); err != nil {
					return false, fmt.Errorf("failed to create hold: %w", err)
				}
				allHolds = append(allHolds, h)
			}

			// Fix the "now" to be well past retention for all candidates.
			fixedNow := baseTime.AddDate(0, 0, retentionDays+120+1)
			gc := &RetentionGC{store: store, now: func() time.Time { return fixedNow }}

			policy := RetentionPolicy{
				RetentionDays: retentionDays,
				Tenant:        "any", // tenant is per-candidate
			}

			for i, ci := range input.Candidates {
				candidate := GCCandidate{
					ObjectKey: fmt.Sprintf("obj-%d", i),
					Tenant:    ci.Tenant,
					CreatedAt: baseTime.AddDate(0, 0, ci.DayOffset),
				}

				shouldDelete, reason := gc.ShouldDelete(candidate, policy)
				covered := isCoveredByActiveHold(candidate, allHolds)

				if covered {
					// Oracle: if active hold covers candidate, must NOT be deleted.
					if shouldDelete {
						return false, fmt.Errorf(
							"candidate %s (tenant=%s, created=%v) covered by active hold but ShouldDelete=true",
							candidate.ObjectKey, candidate.Tenant, candidate.CreatedAt,
						)
					}
					if reason != "held" {
						return false, fmt.Errorf(
							"candidate %s covered by active hold but reason=%q, want 'held'",
							candidate.ObjectKey, reason,
						)
					}
				} else {
					// If retention has expired and no hold, must be deleted.
					expiry := candidate.CreatedAt.Add(time.Duration(retentionDays) * 24 * time.Hour)
					retentionExpired := fixedNow.After(expiry) || fixedNow.Equal(expiry)

					if retentionExpired {
						if !shouldDelete {
							return false, fmt.Errorf(
								"candidate %s (tenant=%s, created=%v) not covered by hold, retention expired, but ShouldDelete=false (reason=%s)",
								candidate.ObjectKey, candidate.Tenant, candidate.CreatedAt, reason,
							)
						}
						if reason != "" {
							return false, fmt.Errorf(
								"candidate %s deletable but reason=%q, want empty",
								candidate.ObjectKey, reason,
							)
						}
					} else {
						// Retention not yet expired — should NOT delete.
						if shouldDelete {
							return false, fmt.Errorf(
								"candidate %s retention not expired but ShouldDelete=true",
								candidate.ObjectKey,
							)
						}
						if reason != "retention_not_expired" {
							return false, fmt.Errorf(
								"candidate %s retention not expired but reason=%q",
								candidate.ObjectKey, reason,
							)
						}
					}
				}
			}
			return true, nil
		},
		genProperty32Input(),
	))

	properties.TestingRun(t)
}
