package auth

import "strings"

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
	RoleAgent  Role = "agent"
)

func NormalizeRole(role string) Role {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case string(RoleAdmin):
		return RoleAdmin
	case string(RoleEditor):
		return RoleEditor
	case string(RoleViewer):
		return RoleViewer
	case string(RoleAgent):
		return RoleAgent
	default:
		return RoleViewer
	}
}

func HasRole(role string, allowed ...Role) bool {
	if len(allowed) == 0 {
		return false
	}
	current := NormalizeRole(role)
	for _, candidate := range allowed {
		if current == candidate {
			return true
		}
	}
	return false
}

func IsAdmin(role string) bool {
	return NormalizeRole(role) == RoleAdmin
}
