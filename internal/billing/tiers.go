package billing

// TierLimits defines the resource limits for a given subscription tier.
// A limit of -1 means unlimited.
type TierLimits struct {
	ProjectLimit        int
	ChunkLimit          int
	ChunkWarn           int  // threshold at which to show the 80% warning; -1 = no warning
	UserLimit           int  // max users in org; -1 = unlimited (Team+ is seat-billed, enforced at billing)
	AgentLimit          int  // max agents in org; -1 = unlimited
	AllowAgents         bool // agents are allowed on all tiers
	AllowAgentMemory    bool // agent memory (memory_enabled) — all tiers, limits do the work
	AllowOrgMemory      bool // org-scoped context — Pro+ only
	OrgMemoryGovernance bool // org memory governance features — Team only
	AllowSSO            bool // SAML/SSO — Enterprise only
	IsTeam              bool
}

var limits = map[string]TierLimits{
	// Free: full product, usage limits do the work. Drive adoption.
	// 1000 chunks disappears fast once identity, conventions, and write_memory are in use.
	"free": {
		ProjectLimit:        1,
		ChunkLimit:          1000,
		ChunkWarn:           800,
		UserLimit:           1,
		AgentLimit:          3,
		AllowAgents:         true,
		AllowAgentMemory:    true,
		AllowOrgMemory:      false,
		OrgMemoryGovernance: false,
		AllowSSO:            false,
	},
	"pro": {
		ProjectLimit:        5,
		ChunkLimit:          10000,
		ChunkWarn:           8000,
		UserLimit:           1,
		AgentLimit:          10,
		AllowAgents:         true,
		AllowAgentMemory:    true,
		AllowOrgMemory:      true,
		OrgMemoryGovernance: false,
		AllowSSO:            false,
	},
	"team": {
		ProjectLimit:        -1,
		ChunkLimit:          -1,
		ChunkWarn:           -1,
		UserLimit:           -1, // seat count enforced at billing level, not here
		AgentLimit:          -1,
		AllowAgents:         true,
		AllowAgentMemory:    true,
		AllowOrgMemory:      true,
		OrgMemoryGovernance: true,
		AllowSSO:            false,
		IsTeam:              true,
	},
	"enterprise": {
		ProjectLimit:        -1,
		ChunkLimit:          -1,
		ChunkWarn:           -1,
		UserLimit:           -1,
		AgentLimit:          -1,
		AllowAgents:         true,
		AllowAgentMemory:    true,
		AllowOrgMemory:      true,
		OrgMemoryGovernance: true,
		AllowSSO:            true,
		IsTeam:              true,
	},
}

// For returns the TierLimits for the given tier string.
// Unknown tiers fall back to free limits.
func For(tier string) TierLimits {
	if l, ok := limits[tier]; ok {
		return l
	}
	return limits["free"]
}
