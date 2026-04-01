package internal

import (
	"testing"
)

func TestNewAuthorizer(t *testing.T) {
	auth := NewAuthorizer()

	if auth == nil {
		t.Fatal("NewAuthorizer returned nil")
	}

	// Should have default roles
	roles := auth.ListRoles()
	if len(roles) == 0 {
		t.Error("Should have default roles")
	}
}

func TestAuthorizer_DefaultRoles(t *testing.T) {
	auth := NewAuthorizer()

	// Check admin role exists
	adminRole, err := auth.GetRole("admin")
	if err != nil {
		t.Fatalf("Admin role not found: %v", err)
	}
	if adminRole.Name != "Administrator" {
		t.Errorf("Expected role name 'Administrator', got %s", adminRole.Name)
	}

	// Check operator role exists
	operatorRole, err := auth.GetRole("operator")
	if err != nil {
		t.Fatalf("Operator role not found: %v", err)
	}
	if operatorRole.Name != "Operator" {
		t.Errorf("Expected role name 'Operator', got %s", operatorRole.Name)
	}

	// Check monitor role exists
	monitorRole, err := auth.GetRole("monitor")
	if err != nil {
		t.Fatalf("Monitor role not found: %v", err)
	}
	if monitorRole.Name != "Monitor" {
		t.Errorf("Expected role name 'Monitor', got %s", monitorRole.Name)
	}
}

func TestAuthorizer_AddRole(t *testing.T) {
	auth := NewAuthorizer()

	customRole := &Role{
		ID:          "custom",
		Name:        "Custom Role",
		Description: "A custom test role",
		Permissions: []*RolePermission{
			{Resource: ResourceSession, Actions: []Action{ActionRead}},
		},
	}

	auth.AddRole(customRole)

	role, err := auth.GetRole("custom")
	if err != nil {
		t.Fatalf("Custom role not found: %v", err)
	}
	if role.Name != "Custom Role" {
		t.Errorf("Expected name 'Custom Role', got %s", role.Name)
	}
}

func TestAuthorizer_RemoveRole(t *testing.T) {
	auth := NewAuthorizer()

	// Add and then remove a role
	auth.AddRole(&Role{ID: "temp", Name: "Temp"})
	auth.RemoveRole("temp")

	_, err := auth.GetRole("temp")
	if err != ErrRoleNotFound {
		t.Error("Role should not exist after removal")
	}
}

func TestAuthorizer_Authorize_AdminAccess(t *testing.T) {
	auth := NewAuthorizer()

	user := &AuthenticatedUser{
		ID:          "admin-user",
		Name:        "Admin User",
		Permissions: []Permission{PermissionAdmin},
	}

	// Admin should have access to everything
	resources := []AuthzResource{ResourceSession, ResourceRecording, ResourceConfig, ResourceUser}
	actions := []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete, ActionManage}

	for _, resource := range resources {
		for _, action := range actions {
			ctx := &AuthzContext{
				User:     user,
				Resource: resource,
				Action:   action,
			}
			err := auth.Authorize(ctx)
			if err != nil {
				t.Errorf("Admin should have access to %s:%s, got %v", resource, action, err)
			}
		}
	}
}

func TestAuthorizer_Authorize_NoAccess(t *testing.T) {
	auth := NewAuthorizer()

	// User with no permissions
	user := &AuthenticatedUser{
		ID:          "limited-user",
		Name:        "Limited User",
		Permissions: []Permission{},
	}

	ctx := &AuthzContext{
		User:     user,
		Resource: ResourceConfig,
		Action:   ActionUpdate,
	}

	err := auth.Authorize(ctx)
	if err == nil {
		t.Error("User with no permissions should be denied")
	}
}

func TestAuthorizer_Authorize_NilUser(t *testing.T) {
	auth := NewAuthorizer()

	ctx := &AuthzContext{
		User:     nil,
		Resource: ResourceSession,
		Action:   ActionRead,
	}

	err := auth.Authorize(ctx)
	if err != ErrAccessDenied {
		t.Errorf("Expected ErrAccessDenied for nil user, got %v", err)
	}
}

func TestAuthorizer_AuthorizeUser(t *testing.T) {
	auth := NewAuthorizer()

	user := &AuthenticatedUser{
		ID:          "admin-user",
		Permissions: []Permission{PermissionAdmin},
	}

	err := auth.AuthorizeUser(user, ResourceSession, ActionRead)
	if err != nil {
		t.Errorf("AuthorizeUser should allow admin access: %v", err)
	}
}

func TestAuthorizer_RoleInheritance(t *testing.T) {
	auth := NewAuthorizer()

	// Recording manager inherits from monitor
	role, err := auth.GetRole("recording_manager")
	if err != nil {
		t.Fatalf("Recording manager role not found: %v", err)
	}

	if len(role.Inherits) == 0 {
		t.Error("Recording manager should inherit from monitor")
	}
	if role.Inherits[0] != "monitor" {
		t.Errorf("Expected to inherit from 'monitor', got %s", role.Inherits[0])
	}

	// Resolve permissions should include inherited ones
	permissions := auth.resolvePermissions("recording_manager")

	// Should have recording permissions
	hasRecordingWrite := false
	for _, p := range permissions {
		if p.Resource == ResourceRecording {
			for _, a := range p.Actions {
				if a == ActionUpdate {
					hasRecordingWrite = true
					break
				}
			}
		}
	}
	if !hasRecordingWrite {
		t.Error("Recording manager should have recording write permission")
	}

	// Should have inherited session read
	hasSessionRead := false
	for _, p := range permissions {
		if p.Resource == ResourceSession {
			for _, a := range p.Actions {
				if a == ActionRead {
					hasSessionRead = true
					break
				}
			}
		}
	}
	if !hasSessionRead {
		t.Error("Recording manager should inherit session read from monitor")
	}
}

func TestPolicy_Evaluate(t *testing.T) {
	policy := &Policy{
		ID:      "test-policy",
		Name:    "Test Policy",
		Enabled: true,
		Rules: []*PolicyRule{
			{
				Effect:    PolicyDeny,
				Resources: []AuthzResource{ResourceConfig},
				Actions:   []Action{ActionDelete},
			},
		},
	}

	// Test deny rule
	ctx := &AuthzContext{
		User:     &AuthenticatedUser{ID: "test-user"},
		Resource: ResourceConfig,
		Action:   ActionDelete,
	}

	decision := policy.Evaluate(ctx)
	if decision != PolicyDeny {
		t.Errorf("Expected PolicyDeny, got %v", decision)
	}

	// Test not applicable
	ctx.Resource = ResourceSession
	decision = policy.Evaluate(ctx)
	if decision != PolicyNotApplicable {
		t.Errorf("Expected PolicyNotApplicable, got %v", decision)
	}
}

func TestPolicy_Disabled(t *testing.T) {
	policy := &Policy{
		ID:      "disabled-policy",
		Enabled: false,
		Rules: []*PolicyRule{
			{
				Effect:    PolicyDeny,
				Resources: []AuthzResource{ResourceConfig},
				Actions:   []Action{ActionDelete},
			},
		},
	}

	ctx := &AuthzContext{
		User:     &AuthenticatedUser{ID: "test-user"},
		Resource: ResourceConfig,
		Action:   ActionDelete,
	}

	decision := policy.Evaluate(ctx)
	if decision != PolicyNotApplicable {
		t.Error("Disabled policy should return NotApplicable")
	}
}

func TestPolicyRule_Matches_Principals(t *testing.T) {
	rule := &PolicyRule{
		Effect:     PolicyAllow,
		Resources:  []AuthzResource{ResourceSession},
		Actions:    []Action{ActionRead},
		Principals: []string{"admin-user", "admin"},
	}

	// User with matching ID
	ctx := &AuthzContext{
		User:     &AuthenticatedUser{ID: "admin-user"},
		Resource: ResourceSession,
		Action:   ActionRead,
	}

	if !rule.Matches(ctx) {
		t.Error("Rule should match user by ID")
	}

	// User with matching permission
	ctx.User = &AuthenticatedUser{
		ID:          "other-user",
		Permissions: []Permission{PermissionAdmin},
	}

	if !rule.Matches(ctx) {
		t.Error("Rule should match user by permission")
	}

	// User without match
	ctx.User = &AuthenticatedUser{
		ID:          "regular-user",
		Permissions: []Permission{PermissionRead},
	}

	if rule.Matches(ctx) {
		t.Error("Rule should not match regular user")
	}
}

func TestPolicyRule_Matches_Wildcard(t *testing.T) {
	rule := &PolicyRule{
		Effect:     PolicyAllow,
		Resources:  []AuthzResource{ResourceHealth},
		Actions:    []Action{ActionRead},
		Principals: []string{"*"},
	}

	ctx := &AuthzContext{
		User:     &AuthenticatedUser{ID: "any-user"},
		Resource: ResourceHealth,
		Action:   ActionRead,
	}

	if !rule.Matches(ctx) {
		t.Error("Rule with wildcard principal should match any user")
	}
}

func TestResourceSpec_Matches(t *testing.T) {
	spec := &ResourceSpec{Pattern: "session/*"}
	err := spec.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	if !spec.Matches("session/123") {
		t.Error("Should match 'session/123'")
	}
	if !spec.Matches("session/abc-xyz") {
		t.Error("Should match 'session/abc-xyz'")
	}
	if spec.Matches("recording/123") {
		t.Error("Should not match 'recording/123'")
	}
}

func TestResourceSpec_Empty(t *testing.T) {
	spec := &ResourceSpec{}
	spec.Compile()

	// Empty spec should match anything
	if !spec.Matches("anything") {
		t.Error("Empty spec should match anything")
	}
}

func TestAuthorizer_AddPolicy(t *testing.T) {
	auth := NewAuthorizer()

	policy := &Policy{
		ID:       "test-policy",
		Name:     "Test Policy",
		Priority: 100,
		Enabled:  true,
	}

	auth.AddPolicy(policy)

	policies := auth.ListPolicies()
	found := false
	for _, p := range policies {
		if p.ID == "test-policy" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Policy not found after adding")
	}
}

func TestAuthorizer_RemovePolicy(t *testing.T) {
	auth := NewAuthorizer()

	policy := &Policy{ID: "to-remove"}
	auth.AddPolicy(policy)
	auth.RemovePolicy("to-remove")

	policies := auth.ListPolicies()
	for _, p := range policies {
		if p.ID == "to-remove" {
			t.Error("Policy should be removed")
		}
	}
}

func TestAuthorizer_PolicyPriority(t *testing.T) {
	auth := NewAuthorizer()

	auth.AddPolicy(&Policy{ID: "low", Priority: 10})
	auth.AddPolicy(&Policy{ID: "high", Priority: 100})
	auth.AddPolicy(&Policy{ID: "medium", Priority: 50})

	policies := auth.ListPolicies()
	if len(policies) < 3 {
		t.Fatal("Expected at least 3 policies")
	}

	// Higher priority should be first
	if policies[0].ID != "high" {
		t.Errorf("Expected 'high' priority first, got %s", policies[0].ID)
	}
}

func TestAuthorizer_PolicyDeny(t *testing.T) {
	auth := NewAuthorizer()

	// Add a deny policy
	auth.AddPolicy(&Policy{
		ID:       "deny-config-delete",
		Priority: 1000,
		Enabled:  true,
		Rules: []*PolicyRule{
			{
				Effect:     PolicyDeny,
				Resources:  []AuthzResource{ResourceConfig},
				Actions:    []Action{ActionDelete},
				Principals: []string{"*"},
			},
		},
	})

	// Even admin should be denied by policy
	user := &AuthenticatedUser{
		ID:          "admin",
		Permissions: []Permission{PermissionAdmin},
	}

	ctx := &AuthzContext{
		User:     user,
		Resource: ResourceConfig,
		Action:   ActionDelete,
	}

	err := auth.Authorize(ctx)
	if err != ErrPolicyViolation {
		t.Errorf("Expected ErrPolicyViolation, got %v", err)
	}
}

func TestAuthorizer_PolicyAllow(t *testing.T) {
	auth := NewAuthorizer()

	// Add an allow policy
	auth.AddPolicy(&Policy{
		ID:       "allow-health-check",
		Priority: 1000,
		Enabled:  true,
		Rules: []*PolicyRule{
			{
				Effect:     PolicyAllow,
				Resources:  []AuthzResource{ResourceHealth},
				Actions:    []Action{ActionRead},
				Principals: []string{"*"},
			},
		},
	})

	// User with no permissions should be allowed by policy
	user := &AuthenticatedUser{
		ID:          "anonymous",
		Permissions: []Permission{},
	}

	ctx := &AuthzContext{
		User:     user,
		Resource: ResourceHealth,
		Action:   ActionRead,
	}

	err := auth.Authorize(ctx)
	if err != nil {
		t.Errorf("Policy should allow health check: %v", err)
	}
}

func TestQuickCheck(t *testing.T) {
	auth := NewAuthorizer()

	// Admin user
	adminUser := &AuthenticatedUser{
		Permissions: []Permission{PermissionAdmin},
	}

	if !auth.QuickCheck(adminUser, ResourceConfig, ActionUpdate) {
		t.Error("Admin should pass QuickCheck")
	}

	// User with read permission
	readUser := &AuthenticatedUser{
		Permissions: []Permission{PermissionRead},
	}

	if !auth.QuickCheck(readUser, ResourceSession, ActionRead) {
		t.Error("User with read permission should pass QuickCheck for session read")
	}

	if auth.QuickCheck(readUser, ResourceConfig, ActionUpdate) {
		t.Error("User with read permission should not pass QuickCheck for config update")
	}

	// Nil user
	if auth.QuickCheck(nil, ResourceSession, ActionRead) {
		t.Error("Nil user should fail QuickCheck")
	}
}

func TestContainsAction(t *testing.T) {
	actions := []Action{ActionRead, ActionCreate}

	if !containsAction(actions, ActionRead) {
		t.Error("Should contain ActionRead")
	}
	if containsAction(actions, ActionDelete) {
		t.Error("Should not contain ActionDelete")
	}

	// Manage action should match any
	manageActions := []Action{ActionManage}
	if !containsAction(manageActions, ActionRead) {
		t.Error("Manage should match any action")
	}
}

func TestCanAccess(t *testing.T) {
	user := &AuthenticatedUser{
		Permissions: []Permission{PermissionAdmin},
	}

	if !CanAccess(user, ResourceSession, ActionRead) {
		t.Error("Admin should have access")
	}

	if CanAccess(nil, ResourceSession, ActionRead) {
		t.Error("Nil user should not have access")
	}
}

func TestFormatPermissions(t *testing.T) {
	user := &AuthenticatedUser{
		Permissions: []Permission{PermissionRead, PermissionWrite},
	}

	result := FormatPermissions(user)
	if result != "read, write" {
		t.Errorf("Expected 'read, write', got %s", result)
	}

	result = FormatPermissions(nil)
	if result != "none" {
		t.Errorf("Expected 'none' for nil user, got %s", result)
	}
}

func TestAuditLogging(t *testing.T) {
	auth := NewAuthorizer()

	var loggedEntry *AuthzAuditEntry
	auth.SetAuditLogger(func(entry *AuthzAuditEntry) {
		loggedEntry = entry
	})

	user := &AuthenticatedUser{
		ID:          "test-user",
		Name:        "Test User",
		Permissions: []Permission{PermissionAdmin},
	}

	ctx := &AuthzContext{
		User:     user,
		Resource: ResourceSession,
		Action:   ActionRead,
	}

	auth.Authorize(ctx)

	if loggedEntry == nil {
		t.Fatal("Audit entry should be logged")
	}
	if loggedEntry.UserID != "test-user" {
		t.Errorf("Expected UserID 'test-user', got %s", loggedEntry.UserID)
	}
	if !loggedEntry.Allowed {
		t.Error("Entry should show allowed")
	}
}

func TestEvaluatePolicyCondition_TimeRange(t *testing.T) {
	cond := &PolicyCondition{
		Type: "time_range",
		Value: map[string]interface{}{
			"start_hour": 0,
			"end_hour":   24,
		},
	}

	ctx := &AuthzContext{}

	// Should always be within 0-24 range
	if !evaluatePolicyCondition(cond, ctx) {
		t.Error("Time range 0-24 should always pass")
	}
}

func TestEvaluatePolicyCondition_Metadata(t *testing.T) {
	cond := &PolicyCondition{
		Type:  "metadata",
		Key:   "tenant_id",
		Value: "tenant-123",
	}

	// With matching metadata
	ctx := &AuthzContext{
		Metadata: map[string]interface{}{
			"tenant_id": "tenant-123",
		},
	}
	if !evaluatePolicyCondition(cond, ctx) {
		t.Error("Should match metadata")
	}

	// Without matching metadata
	ctx.Metadata["tenant_id"] = "tenant-456"
	if evaluatePolicyCondition(cond, ctx) {
		t.Error("Should not match different metadata")
	}

	// Without metadata
	ctx.Metadata = nil
	if evaluatePolicyCondition(cond, ctx) {
		t.Error("Should not match nil metadata")
	}
}
