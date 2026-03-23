package models

import (
	"encoding/json"
	"slices"
	"testing"
)

type InjectAudienceRule struct {
	All            bool     `json:"all,omitempty"`
	AgentTypes     []string `json:"agent_types,omitempty"`
	AgentIDs       []string `json:"agent_ids,omitempty"`
	LifecycleTypes []string `json:"lifecycle_types,omitempty"`
	AgentTags      []string `json:"agent_tags,omitempty"`
	FocusTags      []string `json:"focus_tags,omitempty"`
	OrgIDs         []string `json:"org_ids,omitempty"`
	ProjectIDs     []string `json:"project_ids,omitempty"`
}

type InjectAudience struct {
	Rules []InjectAudienceRule `json:"rules"`
}

// UnmarshalJSON handles both a JSON object (normal) and a double-encoded
// JSON string that MCP clients commonly send. Example inputs:
//
//	{"rules":[{"all":true}]}           → parsed directly
//	"{\"rules\":[{\"all\":true}]}"     → string unwrapped, then parsed
func (ia *InjectAudience) UnmarshalJSON(data []byte) error {
	// Try direct object parse first (fast path).
	type plain InjectAudience
	if err := json.Unmarshal(data, (*plain)(ia)); err == nil {
		return nil
	}

	// Fall back: maybe it's a JSON string containing the object.
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return json.Unmarshal([]byte(s), (*plain)(ia))
}

func (ia *InjectAudience) MatchesSession(agentID, agentType, lifecycleType, orgID, projectID string, agentTags, focusTags []string) bool {
	if ia == nil || len(ia.Rules) == 0 {
		return false
	}
	for _, rule := range ia.Rules {
		if rule.matches(agentID, agentType, lifecycleType, orgID, projectID, agentTags, focusTags) {
			return true
		}
	}
	return false
}

func (r InjectAudienceRule) matches(agentID, agentType, lifecycleType, orgID, projectID string, agentTags, focusTags []string) bool {
	if r.All {
		return true
	}
	if len(r.AgentIDs) > 0 && !slices.Contains(r.AgentIDs, agentID) {
		return false
	}
	if len(r.AgentTypes) > 0 && !slices.Contains(r.AgentTypes, agentType) {
		return false
	}
	if len(r.LifecycleTypes) > 0 && !slices.Contains(r.LifecycleTypes, lifecycleType) {
		return false
	}
	if len(r.OrgIDs) > 0 && !slices.Contains(r.OrgIDs, orgID) {
		return false
	}
	if len(r.ProjectIDs) > 0 && !slices.Contains(r.ProjectIDs, projectID) {
		return false
	}
	if len(r.AgentTags) > 0 && !AnyOverlap(r.AgentTags, agentTags) {
		return false
	}
	if len(r.FocusTags) > 0 && !AnyOverlap(r.FocusTags, focusTags) {
		return false
	}
	return true
}

func AnyOverlap(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

func DefaultInjectAudienceAll() *InjectAudience {
	return &InjectAudience{
		Rules: []InjectAudienceRule{{All: true}},
	}
}

func (ia *InjectAudience) IsInjectable() bool {
	return ia != nil && len(ia.Rules) > 0
}

func TestInjectAudienceRule_AgentTags(t *testing.T) {
	t.Parallel()

	rule := InjectAudienceRule{AgentTags: []string{"backend", "go"}}

	t.Run("match when session agent has overlapping tag", func(t *testing.T) {
		t.Parallel()
		ctx := []string{"go", "senior"}
		if !rule.matches("agent-1", "", "", "", "", ctx, nil) {
			t.Error("should match when session agent has overlapping tag")
		}
	})

	t.Run("no match when no tag overlap", func(t *testing.T) {
		t.Parallel()
		ctx := []string{"frontend", "typescript"}
		if rule.matches("agent-1", "", "", "", "", ctx, nil) {
			t.Error("should not match when no tag overlap")
		}
	})

	t.Run("no match when session agent has no tags", func(t *testing.T) {
		t.Parallel()
		ctx := []string{}
		if rule.matches("agent-1", "", "", "", "", ctx, nil) {
			t.Error("should not match when session agent has no tags")
		}
	})

	t.Run("rule with no agent_tags constraint always matches", func(t *testing.T) {
		t.Parallel()
		emptyRule := InjectAudienceRule{}
		ctx := []string{}
		if !emptyRule.matches("agent-1", "", "", "", "", ctx, nil) {
			t.Error("rule with no agent_tags constraint should always match (unless other constraints fail)")
		}
	})

	t.Run("agent_tags combined with agent_type", func(t *testing.T) {
		t.Parallel()
		combinedRule := InjectAudienceRule{
			AgentTypes: []string{"CODER"},
			AgentTags:  []string{"backend"},
		}
		match := combinedRule.matches("agent-1", "CODER", "", "", "", []string{"backend", "go"}, nil)
		if !match {
			t.Error("should match when both agent_type and agent_tags align")
		}
		noMatch := combinedRule.matches("agent-1", "QA", "", "", "", []string{"backend"}, nil)
		if noMatch {
			t.Error("should not match when agent_type doesn't match even with correct tags")
		}
	})
}

func TestAnyOverlap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		a, b     []string
		expected bool
	}{
		{"both empty", []string{}, []string{}, false},
		{"a empty", []string{}, []string{"x"}, false},
		{"b empty", []string{"x"}, []string{}, false},
		{"overlap", []string{"a", "b"}, []string{"b", "c"}, true},
		{"no overlap", []string{"a"}, []string{"b"}, false},
		{"single match", []string{"x"}, []string{"x"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := AnyOverlap(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("AnyOverlap(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestInjectAudience_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("object input", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"rules":[{"all":true}]}`)
		var ia InjectAudience
		if err := json.Unmarshal(data, &ia); err != nil {
			t.Fatalf("unmarshal object: %v", err)
		}
		if len(ia.Rules) != 1 || !ia.Rules[0].All {
			t.Errorf("rules = %+v, want [{All:true}]", ia.Rules)
		}
	})

	t.Run("double-encoded string input", func(t *testing.T) {
		t.Parallel()
		// This is what MCP clients commonly send — a JSON string containing the object.
		data := []byte(`"{\"rules\":[{\"project_ids\":[\"proj-123\"]}]}"`)
		var ia InjectAudience
		if err := json.Unmarshal(data, &ia); err != nil {
			t.Fatalf("unmarshal string: %v", err)
		}
		if len(ia.Rules) != 1 {
			t.Fatalf("rules length = %d, want 1", len(ia.Rules))
		}
		if len(ia.Rules[0].ProjectIDs) != 1 || ia.Rules[0].ProjectIDs[0] != "proj-123" {
			t.Errorf("project_ids = %v, want [proj-123]", ia.Rules[0].ProjectIDs)
		}
	})

	t.Run("null input", func(t *testing.T) {
		t.Parallel()
		data := []byte(`null`)
		var ia *InjectAudience
		if err := json.Unmarshal(data, &ia); err != nil {
			t.Fatalf("unmarshal null: %v", err)
		}
		if ia != nil {
			t.Errorf("ia = %+v, want nil", ia)
		}
	})
}

func TestInjectAudienceRule_ProjectIDs(t *testing.T) {
	t.Parallel()

	rule := InjectAudienceRule{ProjectIDs: []string{"proj-1", "proj-2"}}

	t.Run("match when session project is in list", func(t *testing.T) {
		t.Parallel()
		if !rule.matches("agent-1", "", "", "", "proj-1", nil, nil) {
			t.Error("should match when project_id is in the rule's project_ids")
		}
	})

	t.Run("no match when session project not in list", func(t *testing.T) {
		t.Parallel()
		if rule.matches("agent-1", "", "", "", "proj-3", nil, nil) {
			t.Error("should not match when project_id is not in the rule's project_ids")
		}
	})

	t.Run("no match when session has no project", func(t *testing.T) {
		t.Parallel()
		if rule.matches("agent-1", "", "", "", "", nil, nil) {
			t.Error("should not match when session has no project_id")
		}
	})
}
