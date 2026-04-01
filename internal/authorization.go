package internal

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"time"
)

// Authorization errors
var (
	ErrAccessDenied       = errors.New("access denied")
	ErrRoleNotFound       = errors.New("role not found")
	ErrPolicyViolation    = errors.New("policy violation")
	ErrResourceNotAllowed = errors.New("resource access not allowed")
)

// Action represents an authorization action
type Action string

const (
	ActionCreate  Action = "create"
	ActionRead    Action = "read"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"
	ActionExecute Action = "execute"
	ActionManage  Action = "manage"
)

// AuthzResource represents a protected resource type
type AuthzResource string

const (
	ResourceSession    AuthzResource = "session"
	ResourceRecording  AuthzResource = "recording"
	ResourceStatistics AuthzResource = "statistics"
	ResourceConfig     AuthzResource = "config"
	ResourceUser       AuthzResource = "user"
	ResourceAPIKey     AuthzResource = "apikey"
	ResourceAuditLog   AuthzResource = "auditlog"
	ResourceHealth     AuthzResource = "health"
	ResourceMetrics    AuthzResource = "metrics"
	ResourceCall       AuthzResource = "call"
	ResourceMedia      AuthzResource = "media"
)

// Role represents an authorization role
type Role struct {
	ID          string
	Name        string
	Description string
	Permissions []*RolePermission
	Inherits    []string // Role IDs to inherit from
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RolePermission defines what a role can do
type RolePermission struct {
	Resource    AuthzResource
	Actions     []Action
	Conditions  []Condition // Optional conditions
	Constraints map[string]interface{}
}

// Condition represents a conditional authorization rule
type Condition interface {
	Evaluate(ctx *AuthzContext) bool
}

// AuthzContext holds context for authorization decisions
type AuthzContext struct {
	User       *AuthenticatedUser
	Resource   AuthzResource
	ResourceID string
	Action     Action
	Metadata   map[string]interface{}
	RequestCtx context.Context
}

// Authorizer handles authorization decisions
type Authorizer struct {
	roles    map[string]*Role
	policies []*Policy
	mu       sync.RWMutex

	// Cache for role inheritance resolution
	inheritanceCache map[string][]*RolePermission
	cacheMu          sync.RWMutex

	// Audit logging
	auditLog func(entry *AuthzAuditEntry)
}

// AuthorizerConfig holds authorizer configuration
type AuthorizerConfig struct {
	EnableAuditLog bool
	DefaultDeny    bool
}

// NewAuthorizer creates a new authorizer
func NewAuthorizer() *Authorizer {
	auth := &Authorizer{
		roles:            make(map[string]*Role),
		policies:         make([]*Policy, 0),
		inheritanceCache: make(map[string][]*RolePermission),
	}

	// Add default roles
	auth.addDefaultRoles()

	return auth
}

// addDefaultRoles adds common roles
func (a *Authorizer) addDefaultRoles() {
	// Admin role - full access
	a.AddRole(&Role{
		ID:          "admin",
		Name:        "Administrator",
		Description: "Full administrative access",
		Permissions: []*RolePermission{
			{Resource: ResourceSession, Actions: []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete, ActionManage}},
			{Resource: ResourceRecording, Actions: []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete, ActionManage}},
			{Resource: ResourceStatistics, Actions: []Action{ActionRead}},
			{Resource: ResourceConfig, Actions: []Action{ActionRead, ActionUpdate}},
			{Resource: ResourceUser, Actions: []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}},
			{Resource: ResourceAPIKey, Actions: []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}},
			{Resource: ResourceAuditLog, Actions: []Action{ActionRead}},
			{Resource: ResourceHealth, Actions: []Action{ActionRead}},
			{Resource: ResourceMetrics, Actions: []Action{ActionRead}},
			{Resource: ResourceCall, Actions: []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete, ActionManage}},
			{Resource: ResourceMedia, Actions: []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete, ActionManage}},
		},
	})

	// Operator role - operational access
	a.AddRole(&Role{
		ID:          "operator",
		Name:        "Operator",
		Description: "Operational access for managing calls and sessions",
		Permissions: []*RolePermission{
			{Resource: ResourceSession, Actions: []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}},
			{Resource: ResourceRecording, Actions: []Action{ActionCreate, ActionRead}},
			{Resource: ResourceStatistics, Actions: []Action{ActionRead}},
			{Resource: ResourceHealth, Actions: []Action{ActionRead}},
			{Resource: ResourceMetrics, Actions: []Action{ActionRead}},
			{Resource: ResourceCall, Actions: []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}},
			{Resource: ResourceMedia, Actions: []Action{ActionCreate, ActionRead, ActionUpdate}},
		},
	})

	// Monitor role - read-only access
	a.AddRole(&Role{
		ID:          "monitor",
		Name:        "Monitor",
		Description: "Read-only access for monitoring",
		Permissions: []*RolePermission{
			{Resource: ResourceSession, Actions: []Action{ActionRead}},
			{Resource: ResourceRecording, Actions: []Action{ActionRead}},
			{Resource: ResourceStatistics, Actions: []Action{ActionRead}},
			{Resource: ResourceHealth, Actions: []Action{ActionRead}},
			{Resource: ResourceMetrics, Actions: []Action{ActionRead}},
			{Resource: ResourceCall, Actions: []Action{ActionRead}},
		},
	})

	// Recording manager role
	a.AddRole(&Role{
		ID:          "recording_manager",
		Name:        "Recording Manager",
		Description: "Manage call recordings",
		Inherits:    []string{"monitor"},
		Permissions: []*RolePermission{
			{Resource: ResourceRecording, Actions: []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete}},
		},
	})
}

// AddRole adds or updates a role
func (a *Authorizer) AddRole(role *Role) {
	a.mu.Lock()
	defer a.mu.Unlock()

	role.UpdatedAt = time.Now()
	if role.CreatedAt.IsZero() {
		role.CreatedAt = time.Now()
	}

	a.roles[role.ID] = role

	// Invalidate cache
	a.cacheMu.Lock()
	delete(a.inheritanceCache, role.ID)
	a.cacheMu.Unlock()
}

// GetRole retrieves a role by ID
func (a *Authorizer) GetRole(id string) (*Role, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	role, exists := a.roles[id]
	if !exists {
		return nil, ErrRoleNotFound
	}
	return role, nil
}

// RemoveRole removes a role
func (a *Authorizer) RemoveRole(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.roles, id)

	// Invalidate cache
	a.cacheMu.Lock()
	delete(a.inheritanceCache, id)
	a.cacheMu.Unlock()
}

// ListRoles returns all roles
func (a *Authorizer) ListRoles() []*Role {
	a.mu.RLock()
	defer a.mu.RUnlock()

	roles := make([]*Role, 0, len(a.roles))
	for _, role := range a.roles {
		roles = append(roles, role)
	}
	return roles
}

// resolvePermissions resolves all permissions for a role including inherited ones
func (a *Authorizer) resolvePermissions(roleID string) []*RolePermission {
	// Check cache
	a.cacheMu.RLock()
	if cached, exists := a.inheritanceCache[roleID]; exists {
		a.cacheMu.RUnlock()
		return cached
	}
	a.cacheMu.RUnlock()

	a.mu.RLock()
	role, exists := a.roles[roleID]
	a.mu.RUnlock()

	if !exists {
		return nil
	}

	// Collect permissions from inherited roles first
	var permissions []*RolePermission
	seen := make(map[string]bool)
	seen[roleID] = true

	for _, inheritedID := range role.Inherits {
		if !seen[inheritedID] {
			seen[inheritedID] = true
			permissions = append(permissions, a.resolvePermissions(inheritedID)...)
		}
	}

	// Add this role's permissions
	permissions = append(permissions, role.Permissions...)

	// Cache result
	a.cacheMu.Lock()
	a.inheritanceCache[roleID] = permissions
	a.cacheMu.Unlock()

	return permissions
}

// Authorize checks if an action is allowed
func (a *Authorizer) Authorize(ctx *AuthzContext) error {
	if ctx.User == nil {
		return ErrAccessDenied
	}

	// Check policies first
	for _, policy := range a.policies {
		decision := policy.Evaluate(ctx)
		if decision == PolicyDeny {
			a.logAudit(ctx, false, "policy_deny")
			return ErrPolicyViolation
		}
		if decision == PolicyAllow {
			a.logAudit(ctx, true, "policy_allow")
			return nil
		}
		// PolicyNotApplicable - continue checking
	}

	// Check role-based permissions
	for _, perm := range ctx.User.Permissions {
		roleID := string(perm) // Permission maps to role ID

		permissions := a.resolvePermissions(roleID)
		for _, p := range permissions {
			if p.Resource == ctx.Resource && containsAction(p.Actions, ctx.Action) {
				// Check conditions
				if len(p.Conditions) > 0 {
					allConditionsMet := true
					for _, cond := range p.Conditions {
						if !cond.Evaluate(ctx) {
							allConditionsMet = false
							break
						}
					}
					if !allConditionsMet {
						continue
					}
				}

				a.logAudit(ctx, true, "role_permission")
				return nil
			}
		}
	}

	// Check if user has admin permission (special case)
	if ctx.User.HasPermission(PermissionAdmin) {
		a.logAudit(ctx, true, "admin_override")
		return nil
	}

	a.logAudit(ctx, false, "no_permission")
	return ErrAccessDenied
}

// AuthorizeUser is a convenience method
func (a *Authorizer) AuthorizeUser(user *AuthenticatedUser, resource AuthzResource, action Action) error {
	return a.Authorize(&AuthzContext{
		User:     user,
		Resource: resource,
		Action:   action,
	})
}

// containsAction checks if an action is in the list
func containsAction(actions []Action, action Action) bool {
	for _, a := range actions {
		if a == action || a == ActionManage {
			return true
		}
	}
	return false
}

// logAudit logs an authorization decision
func (a *Authorizer) logAudit(ctx *AuthzContext, allowed bool, reason string) {
	if a.auditLog == nil {
		return
	}

	entry := &AuthzAuditEntry{
		Timestamp:  time.Now(),
		UserID:     ctx.User.ID,
		UserName:   ctx.User.Name,
		Resource:   ctx.Resource,
		ResourceID: ctx.ResourceID,
		Action:     ctx.Action,
		Allowed:    allowed,
		Reason:     reason,
	}

	a.auditLog(entry)
}

// SetAuditLogger sets the audit log function
func (a *Authorizer) SetAuditLogger(logger func(entry *AuthzAuditEntry)) {
	a.auditLog = logger
}

// AuthzAuditEntry represents an authorization audit log entry
type AuthzAuditEntry struct {
	Timestamp  time.Time
	UserID     string
	UserName   string
	Resource   AuthzResource
	ResourceID string
	Action     Action
	Allowed    bool
	Reason     string
	Metadata   map[string]interface{}
}

// Policy represents an authorization policy
type Policy struct {
	ID          string
	Name        string
	Description string
	Priority    int // Higher priority evaluated first
	Rules       []*PolicyRule
	Enabled     bool
}

// PolicyDecision represents a policy evaluation result
type PolicyDecision int

const (
	PolicyNotApplicable PolicyDecision = iota
	PolicyAllow
	PolicyDeny
)

// PolicyRule represents a single policy rule
type PolicyRule struct {
	Effect       PolicyDecision
	Resources    []AuthzResource
	Actions      []Action
	Principals   []string // User IDs or role IDs
	Conditions   []PolicyCondition
	ResourceSpec *ResourceSpec // For pattern matching
}

// ResourceSpec defines resource matching patterns
type ResourceSpec struct {
	Pattern string         // e.g., "session/*", "recording/call-*"
	regex   *regexp.Regexp // Compiled pattern
}

// Compile compiles the resource pattern
func (rs *ResourceSpec) Compile() error {
	if rs.Pattern == "" {
		return nil
	}

	// Convert glob-like pattern to regex
	pattern := regexp.QuoteMeta(rs.Pattern)
	pattern = "^" + pattern + "$"
	pattern = regexp.MustCompile(`\\\*`).ReplaceAllString(pattern, ".*")

	var err error
	rs.regex, err = regexp.Compile(pattern)
	return err
}

// Matches checks if a resource ID matches the spec
func (rs *ResourceSpec) Matches(resourceID string) bool {
	if rs.regex == nil {
		return true
	}
	return rs.regex.MatchString(resourceID)
}

// PolicyCondition is a policy condition
type PolicyCondition struct {
	Type  string
	Key   string
	Value interface{}
}

// Evaluate evaluates the policy against the context
func (p *Policy) Evaluate(ctx *AuthzContext) PolicyDecision {
	if !p.Enabled {
		return PolicyNotApplicable
	}

	for _, rule := range p.Rules {
		if rule.Matches(ctx) {
			return rule.Effect
		}
	}

	return PolicyNotApplicable
}

// Matches checks if a rule matches the context
func (r *PolicyRule) Matches(ctx *AuthzContext) bool {
	// Check resource
	resourceMatches := false
	for _, res := range r.Resources {
		if res == ctx.Resource {
			resourceMatches = true
			break
		}
	}
	if !resourceMatches && len(r.Resources) > 0 {
		return false
	}

	// Check action
	actionMatches := false
	for _, act := range r.Actions {
		if act == ctx.Action {
			actionMatches = true
			break
		}
	}
	if !actionMatches && len(r.Actions) > 0 {
		return false
	}

	// Check principals
	if len(r.Principals) > 0 {
		principalMatches := false
		for _, principal := range r.Principals {
			if principal == ctx.User.ID || principal == "*" {
				principalMatches = true
				break
			}
			// Check if any permission matches
			for _, perm := range ctx.User.Permissions {
				if principal == string(perm) {
					principalMatches = true
					break
				}
			}
		}
		if !principalMatches {
			return false
		}
	}

	// Check resource spec
	if r.ResourceSpec != nil {
		if !r.ResourceSpec.Matches(ctx.ResourceID) {
			return false
		}
	}

	// Check conditions
	for _, cond := range r.Conditions {
		if !evaluatePolicyCondition(&cond, ctx) {
			return false
		}
	}

	return true
}

// evaluatePolicyCondition evaluates a single policy condition
func evaluatePolicyCondition(cond *PolicyCondition, ctx *AuthzContext) bool {
	switch cond.Type {
	case "time_range":
		// Check if current time is within range
		now := time.Now()
		if rangeMap, ok := cond.Value.(map[string]interface{}); ok {
			if startHour, ok := rangeMap["start_hour"].(int); ok {
				if now.Hour() < startHour {
					return false
				}
			}
			if endHour, ok := rangeMap["end_hour"].(int); ok {
				if now.Hour() >= endHour {
					return false
				}
			}
		}
		return true

	case "ip_range":
		// Check if client IP is in range
		if ctx.Metadata != nil {
			if clientIP, ok := ctx.Metadata["client_ip"].(string); ok {
				if allowedIPs, ok := cond.Value.([]string); ok {
					for _, allowed := range allowedIPs {
						if clientIP == allowed {
							return true
						}
					}
					return false
				}
			}
		}
		return true

	case "metadata":
		// Check metadata key/value
		if ctx.Metadata == nil {
			return false
		}
		if val, ok := ctx.Metadata[cond.Key]; ok {
			return val == cond.Value
		}
		return false

	default:
		return true
	}
}

// AddPolicy adds a policy
func (a *Authorizer) AddPolicy(policy *Policy) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.policies = append(a.policies, policy)

	// Sort by priority (higher first)
	for i := len(a.policies) - 1; i > 0; i-- {
		if a.policies[i].Priority > a.policies[i-1].Priority {
			a.policies[i], a.policies[i-1] = a.policies[i-1], a.policies[i]
		}
	}
}

// RemovePolicy removes a policy by ID
func (a *Authorizer) RemovePolicy(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, p := range a.policies {
		if p.ID == id {
			a.policies = append(a.policies[:i], a.policies[i+1:]...)
			return
		}
	}
}

// ListPolicies returns all policies
func (a *Authorizer) ListPolicies() []*Policy {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]*Policy{}, a.policies...)
}

// ResourcePermissions defines what permissions are needed for resources
type ResourcePermissions struct {
	Create  Permission
	Read    Permission
	Update  Permission
	Delete  Permission
	Manage  Permission
}

// DefaultResourcePermissions maps resources to required permissions
var DefaultResourcePermissions = map[AuthzResource]ResourcePermissions{
	ResourceSession: {
		Create: PermissionWrite,
		Read:   PermissionRead,
		Update: PermissionWrite,
		Delete: PermissionWrite,
		Manage: PermissionAdmin,
	},
	ResourceRecording: {
		Create: PermissionRecording,
		Read:   PermissionRead,
		Update: PermissionRecording,
		Delete: PermissionAdmin,
		Manage: PermissionAdmin,
	},
	ResourceStatistics: {
		Read: PermissionStatistics,
	},
	ResourceConfig: {
		Read:   PermissionAdmin,
		Update: PermissionAdmin,
	},
}

// QuickCheck performs a quick permission check without full policy evaluation
func (a *Authorizer) QuickCheck(user *AuthenticatedUser, resource AuthzResource, action Action) bool {
	if user == nil {
		return false
	}

	// Admin always allowed
	if user.HasPermission(PermissionAdmin) {
		return true
	}

	// Check default resource permissions
	if perms, ok := DefaultResourcePermissions[resource]; ok {
		var requiredPerm Permission
		switch action {
		case ActionCreate:
			requiredPerm = perms.Create
		case ActionRead:
			requiredPerm = perms.Read
		case ActionUpdate:
			requiredPerm = perms.Update
		case ActionDelete:
			requiredPerm = perms.Delete
		case ActionManage:
			requiredPerm = perms.Manage
		}

		if requiredPerm != "" && user.HasPermission(requiredPerm) {
			return true
		}
	}

	return false
}

// AuthzMiddleware creates an authorization middleware for HTTP handlers
func AuthzMiddleware(authorizer *Authorizer, resource AuthzResource, action Action) func(next func(w interface{}, r interface{})) func(w interface{}, r interface{}) {
	return func(next func(w interface{}, r interface{})) func(w interface{}, r interface{}) {
		return func(w interface{}, r interface{}) {
			// This is a simplified middleware - in practice you'd use http.Handler
			next(w, r)
		}
	}
}

// CanAccess is a simple helper for template/UI usage
func CanAccess(user *AuthenticatedUser, resource AuthzResource, action Action) bool {
	auth := NewAuthorizer()
	return auth.QuickCheck(user, resource, action)
}

// FormatPermissions returns a human-readable list of permissions
func FormatPermissions(user *AuthenticatedUser) string {
	if user == nil {
		return "none"
	}

	result := ""
	for i, p := range user.Permissions {
		if i > 0 {
			result += ", "
		}
		result += string(p)
	}
	return result
}
