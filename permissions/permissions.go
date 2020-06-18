package permissions

import (
	"fmt"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/gorm-adapter/v2"
	"github.com/jinzhu/gorm"
	"gitlab.com/ignitionrobotics/web/ign-go"
	"regexp"
)

// Action - type int
type Action int

// Role - type int
type Role int

// A list of actions that can be performed
const (
	// Read-only
	Read Action = iota
	// Write
	Write
)

// Corresponding string value for an Action
var actionStr = []string{"read", "write"}

// String function will return the english name of the Action
func (a Action) String() string {
	return actionStr[a]
}

// ActionFrom returns the Action value corresponding to the given string. It will
// return -1 if not found.
func ActionFrom(str string) Action {
	if actionStr[0] == str {
		return Read
	} else if actionStr[1] == str {
		return Write
	}
	return -1
}

// A list of roles
const (
	// System admin role
	SystemAdmin Role = iota
	// Owner role
	Owner
	// Admin role
	Admin
	// Member role
	Member
)

// Corresponding string value for a Role
var roleStr = []string{"sysadmin", "owner", "admin", "member"}

// String function will return the english name of the Role
func (r Role) String() string {
	return roleStr[r]
}

// RoleFrom returns the Role value corresponding to the given string. It will
// return -1 if not found.
func RoleFrom(str string) (Role, *ign.ErrMsg) {
	if roleStr[0] == str {
		return SystemAdmin, nil
	} else if roleStr[1] == str {
		return Owner, nil
	} else if roleStr[2] == str {
		return Admin, nil
	} else if roleStr[3] == str {
		return Member, nil
	}
	return -1, ign.NewErrorMessageWithArgs(ign.ErrorNameNotFound, nil, []string{"role:", str})
}

const (
	// PolicyUser is the index of 'user' in a casbin policy tuple
	PolicyUser = iota
	// PolicyResource is the index of 'resource' in a casbin policy tuple
	PolicyResource
	// PolicyAction is the index of 'action' in a casbin policy tuple
	PolicyAction
)

// Permissions struct contains a data object for interfacing with permissions db
type Permissions struct {
	data *permissionsObj
}

///////////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////

// Enforcer is the interface that matches all casbin enforcer implementations
// used by ign. It was created to allow other backends to use Permissions passing
// their own Enforcer.
type Enforcer interface {
	LoadPolicy() error
	Enforce(rvals ...interface{}) bool
	DeleteUser(user string) bool
	DeleteRole(role string)
	DeletePermission(permission ...string) bool
	DeleteRolesForUser(user string) bool
	DeleteRoleForUser(user string, role string) bool
	AddRoleForUser(user string, role string) bool
	AddPermissionForUser(user string, permission ...string) bool
	DeletePermissionForUser(user string, permission ...string) bool
	DeletePermissionsForUser(user string) bool
	GetUsersForRole(name string) []string
	GetRolesForUser(name string) []string
	HasRoleForUser(name string, role string) bool
	HasPermissionForUser(user string, permission ...string) bool
	RemoveFilteredPolicy(fieldIndex int, fieldValues ...string) bool
}

///////////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////

// Private permission data objects
type permissionsObj struct {
	adapter  *gormadapter.Adapter
	enforcer *casbin.Enforcer
}

// Global permission object
var gPermissionsObj *permissionsObj

// Init initializes permissions with an existing database connection
func (p *Permissions) Init(db *gorm.DB, sysAdmin string) error {

	// check if db connection and permission policy has been initialized or not
	if gPermissionsObj != nil {
		return nil
	}

	var adapter *gormadapter.Adapter

	adapter, _ = gormadapter.NewAdapterByDB(db)
	enforcer, _ := casbin.NewEnforcer("permissions/policy.conf", adapter)

	return p.InitWithEnforcerAndAdapter(enforcer, adapter, sysAdmin)
}

// InitWithEnforcerAndAdapter initializes permissions with a given pair of
// enforcer and adapter.
func (p *Permissions) InitWithEnforcerAndAdapter(e *casbin.Enforcer, a *gormadapter.Adapter, sysAdmin string) error {

	obj := &permissionsObj{
		enforcer: e,
		adapter:  a,
	}
	gPermissionsObj = obj
	p.data = gPermissionsObj

	p.Reload(sysAdmin)
	return nil
}

// Reload reloads all casbin data
// sysAdmin argument can contain a list of usernames separated by comma.
func (p *Permissions) Reload(sysAdmin string) error {
	// Load the policy from DB.
	p.data.enforcer.LoadPolicy()
	p.setSystemAdmin(sysAdmin)
	return nil
}

// setSystemAdmin configures the system admin(s).
// sysAdmin argument can contain a list of usernames separated by comma.
func (p *Permissions) setSystemAdmin(sysAdmin string) {
	saRole := roleToString(SystemAdmin)
	p.data.enforcer.DeleteRole(saRole)
	if sysAdmin != "" {
		users := ign.StrToSlice(sysAdmin)
		for _, u := range users {
			p.AddRoleForUser(u, saRole)
		}
	}
}

// IsSystemAdmin returns a bool indicating if the given user is a system admin.
func (p *Permissions) IsSystemAdmin(user string) bool {
	result, _ := p.data.enforcer.HasRoleForUser(user, roleToString(SystemAdmin))
	return result
}

// IsAuthorized checks if user has the permission to perform an action on a
// resource
func (p *Permissions) IsAuthorized(user, resource string, action Action) (bool, *ign.ErrMsg) {
	if p.IsSystemAdmin(user) {
		return true, nil
	}

	valid, err := p.data.enforcer.Enforce(user, resource, actionToString(action))
	if !valid || err != nil {
		return false, ign.NewErrorMessage(ign.ErrorUnauthorized)
	}
	return true, nil
}

// AddPermission adds a user (or group) permission on a resource
func (p *Permissions) AddPermission(user, resource string, action Action) (bool, *ign.ErrMsg) {
	valid, err := p.data.enforcer.AddPermissionForUser(user, resource, actionToString(action))
	if !valid || err != nil {
		return false, ign.NewErrorMessage(ign.ErrorUnexpected)
	}
	return true, nil
}

// RemovePermission removes a user (or group) permission on a resource
func (p *Permissions) RemovePermission(user, resource string, action Action) (bool, *ign.ErrMsg) {
	valid, err := p.data.enforcer.DeletePermissionForUser(user, resource, actionToString(action))
	if !valid || err != nil {
		return false, ign.NewErrorMessage(ign.ErrorUnexpected)
	}
	return true, nil
}

// RemoveResource removes a resource and all policies involving the resource
func (p *Permissions) RemoveResource(resource string) (bool, *ign.ErrMsg) {
	// policy is formatted in casbin as (user, resource, action)
	// so the 1 in the arg below means resource.
	valid, err := p.data.enforcer.RemoveFilteredPolicy(PolicyResource, resource)
	if !valid || err != nil {
		return false, ign.NewErrorMessage(ign.ErrorUnexpected)
	}
	return true, nil
}

// GetGroupsForUser returns the list of groups a user belongs to.
func (p *Permissions) GetGroupsForUser(user string) []string {
	groups := make([]string, 0)
	m := p.GetGroupsAndRolesForUser(user)
	for k := range m {
		groups = append(groups, k)
	}
	return groups
}

// GetGroupsAndRolesForUser gets the groups and roles that a user has, in the form of a map with
// groups as keys and the user role in those groups as values.
func (p *Permissions) GetGroupsAndRolesForUser(user string) map[string]string {
	m := make(map[string]string, 0)
	roles, _ := p.data.enforcer.GetRolesForUser(user)
	re := regexp.MustCompile("(.+)_#([^_]+)$")
	for _, r := range roles {
		s := re.FindStringSubmatch(r)
		if s != nil {
			group := s[1]
			role := s[2]
			m[group] = role
		}
	}
	return m
}

// GetUsersForGroup gets the users that belong to a group.
func (p *Permissions) GetUsersForGroup(group string) []string {
	result, _ := p.data.enforcer.GetUsersForRole(group)
	return result
}

// UserBelongsToGroup returns true if the user belongs to the group.
func (p *Permissions) UserBelongsToGroup(user, group string) bool {
	return p.HasRoleForUser(user, group)
}

// HasRoleForUser checks and see if a user has the specified role
func (p *Permissions) HasRoleForUser(user, role string) bool {
	result, _ := p.data.enforcer.HasRoleForUser(user, role)
	return result
}

// AddUserGroupRoleString is same as AddUserGroupRole but receives a role name
// as a string. It will fail if the role name is not 'owner', 'admin' or 'member'.
func (p *Permissions) AddUserGroupRoleString(user, group, role string) (bool, *ign.ErrMsg) {
	r, em := RoleFrom(role)
	if em != nil {
		return false, em
	}
	return p.AddUserGroupRole(user, group, r)
}

// AddUserGroupRole adds a role for a user in a group
func (p *Permissions) AddUserGroupRole(user, group string, role Role) (bool, *ign.ErrMsg) {
	// add user to the group
	// do this by adding user to a role
	ok, em := p.AddRoleForUser(user, group)
	if !ok {
		return ok, em
	}

	// give the user a specific role in the group
	groupRole := getRoleForGroup(role, group)
	ok, em = p.AddRoleForUser(user, groupRole)
	if !ok {
		return ok, em
	}

	// set the role permissions of this group
	return p.SetRolePermissions(group)
}

// AddRoleForUser adds a role for a user
func (p *Permissions) AddRoleForUser(user, role string) (bool, *ign.ErrMsg) {
	valid, _ := p.data.enforcer.HasRoleForUser(user, role)
	if valid {
		extra := fmt.Sprintf("Role [%s] exist for user [%s]", role, user)
		return false, ign.NewErrorMessageWithArgs(ign.ErrorResourceExists, nil, []string{extra})
	}

	added, _ := p.data.enforcer.AddRoleForUser(user, role)
	if !added {
		extra := fmt.Sprintf("Could not add role [%s] for user [%s]", role, user)
		return false, ign.NewErrorMessageWithArgs(ign.ErrorUnexpected, nil, []string{extra})
	}
	return true, nil
}

// RemoveUserGroupRole removes a role from a user in a group
func (p *Permissions) RemoveUserGroupRole(user, group string, role Role) (bool, *ign.ErrMsg) {
	return p.RemoveRoleForUser(user, getRoleForGroup(role, group))
}

// RemoveRoleForUser removes a role from a user
func (p *Permissions) RemoveRoleForUser(user, role string) (bool, *ign.ErrMsg) {
	valid, err := p.data.enforcer.DeleteRoleForUser(user, role)
	if !valid || err != nil {
		return false, ign.NewErrorMessage(ign.ErrorUnexpected)
	}
	return true, nil
}

// GetUserRoleForGroup returns the role of a user in a group. If the user does
// not belong to the group then returns an error.
func (p *Permissions) GetUserRoleForGroup(user, group string) (Role, *ign.ErrMsg) {

	if p.IsSystemAdmin(user) {
		return SystemAdmin, nil
	}
	for _, r := range roleStr {
		role, em := RoleFrom(r)
		if em != nil {
			return -1, em
		}
		groupRole := getRoleForGroup(role, group)
		result, _ := p.data.enforcer.HasRoleForUser(user, groupRole)
		if result {
			return role, nil
		}
	}
	return -1, ign.NewErrorMessage(ign.ErrorUnauthorized)
}

// IsAuthorizedForRole returns true if the user is authorized to act as the given
// role (or above) in the group. Eg. A group  Owner can act as Admin. But a Member
// cannot.
func (p *Permissions) IsAuthorizedForRole(user, group string, role Role) (bool, *ign.ErrMsg) {
	ur, em := p.GetUserRoleForGroup(user, group)
	if em != nil {
		return false, em
	}
	if p.CompareRoles(ur, role) >= 0 {
		return true, nil
	}
	return false, ign.NewErrorMessage(ign.ErrorUnauthorized)
}

// CompareRoles compares the the given roles following this order:
// SystemAdmin > Owner > Admin > Member.
// It returns a positive number if role1 has more privileges than role2. A zero
// value if they are equal, and a negative value otherwise.
func (p *Permissions) CompareRoles(role1, role2 Role) int {
	return int(role2 - role1)
}

// RemoveUserFromGroup removes all roles from a user in a group
func (p *Permissions) RemoveUserFromGroup(user, group string) (bool, *ign.ErrMsg) {
	result, _ := p.data.enforcer.HasRoleForUser(user, group)
	if !result {
		extra := fmt.Sprintf("User [%s] does not belong to group [%s]", user, group)
		return false, ign.NewErrorMessageWithArgs(ign.ErrorNameNotFound, nil, []string{extra})
	}

	// Should not be able to remove the last owner.
	ownerRole := getRoleForGroup(Owner, group)
	owners, _ := p.data.enforcer.GetUsersForRole(ownerRole)
	result, _ = p.data.enforcer.HasRoleForUser(user, ownerRole)
	if len(owners) == 1 && result {
		extra := fmt.Sprintf("Cannot remove the last owner [%s] of an Org [%s]", user, group)
		return false, ign.NewErrorMessageWithArgs(ign.ErrorUnexpected, nil, []string{extra})
	}

	// OK let's remove all roles for this user in the org

	// remove specific group roles of user (owner, admin, member)
	role := getRoleForGroup(Member, group)
	result, _ = p.data.enforcer.HasRoleForUser(user, role)
	if result {
		result, _ = p.data.enforcer.DeleteRoleForUser(user, role)
		if !result {
			return false, ign.NewErrorMessage(ign.ErrorUnexpected)
		}
	}
	role = getRoleForGroup(Admin, group)
	result, _ = p.data.enforcer.HasRoleForUser(user, role)
	if result {
		result, _ = p.data.enforcer.DeleteRoleForUser(user, role)
		if !result {
			return false, ign.NewErrorMessage(ign.ErrorUnexpected)
		}
	}

	result, _ = p.data.enforcer.HasRoleForUser(user, ownerRole)
	// if the user was an owner, then remove that role too.
	if result {
		result, _ = p.data.enforcer.DeleteRoleForUser(user, ownerRole)
		if !result {
			return false, ign.NewErrorMessage(ign.ErrorUnexpected)
		}
	}

	result, _ = p.data.enforcer.DeleteRoleForUser(user, group)
	// finally, remove the user from the group too
	if !result {
		return false, ign.NewErrorMessage(ign.ErrorUnexpected)
	}
	return true, nil
}

// SetRolePermissions sets up role permissions for a group
func (p *Permissions) SetRolePermissions(group string) (bool, *ign.ErrMsg) {

	groupRole := getRoleForGroup(Owner, group)
	// check if permissions have already been set or not
	if p.data.enforcer.HasPermissionForUser(groupRole) {
		return true, nil
	}
	p.AddPermission(groupRole, group, Read)
	p.AddPermission(groupRole, group, Write)

	groupRole = getRoleForGroup(Admin, group)
	p.AddPermission(groupRole, group, Read)
	p.AddPermission(groupRole, group, Write)

	groupRole = getRoleForGroup(Member, group)
	p.AddPermission(groupRole, group, Read)

	return true, nil
}

// RemoveRolePermissions removes role permissions associated with a group
func (p *Permissions) RemoveRolePermissions(group string) (bool, *ign.ErrMsg) {

	groupRole := getRoleForGroup(Owner, group)
	// check if permissions were previously set
	if !p.data.enforcer.HasPermissionForUser(groupRole) {
		return true, nil
	}
	p.RemovePermission(groupRole, group, Read)
	p.RemovePermission(groupRole, group, Write)

	groupRole = getRoleForGroup(Admin, group)
	p.RemovePermission(groupRole, group, Read)
	p.RemovePermission(groupRole, group, Write)

	groupRole = getRoleForGroup(Member, group)
	p.RemovePermission(groupRole, group, Read)

	return true, nil
}

// RemoveUser removes all policies involving the user
func (p *Permissions) RemoveUser(user string) (bool, *ign.ErrMsg) {
	// remove user resource permissions
	p.data.enforcer.DeleteUser(user)
	// remove user roles
	p.data.enforcer.DeletePermissionsForUser(user)
	// the return results are not used as they don't necessarily mean
	// removal failed. A false value may just mean that the user has no
	// permissions or roles
	return true, nil
}

// RemoveGroup removes a role in a group. This should remove all policies
// involving the role
func (p *Permissions) RemoveGroup(group string) (bool, *ign.ErrMsg) {
	p.RemoveRolePermissions(group)
	return p.RemoveRole(group)
}

// RemoveRole removes all policies involving the role
func (p *Permissions) RemoveRole(role string) (bool, *ign.ErrMsg) {
	// casbin does not return a value for deleting roles
	p.data.enforcer.DeleteRole(role)
	return true, nil
}

// DBTable returns the DB table used by casbin
func (p *Permissions) DBTable() *gormadapter.CasbinRule {
	return &gormadapter.CasbinRule{}
}

// Convert Action type to string
func actionToString(action Action) string {
	return action.String()
}

// Convert Action type to string
func roleToString(role Role) string {
	return role.String()
}

// get the string representing a specific role of a group
func getRoleForGroup(role Role, group string) string {
	return group + "_#" + roleToString(role)
}
