package main

import (
	"github.com/gazebo-web/fuel-server/globals"
	"github.com/gazebo-web/fuel-server/permissions"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/stretchr/testify/assert"
	"testing"
)

type userResourcePermissionsTest struct {
	// description of the test
	testDesc string

	// username
	user string

	// resource name
	resource string

	// type of action
	action permissions.Action

	// expected permission result
	expAuthorized bool

	// expected error message
	expErrMsg *gz.ErrMsg
}

// TestPermissionsSetSystemAdmin test configuring system admins
func TestPermissionsSetSystemAdmin(t *testing.T) {

	setup()

	unauth := gz.NewErrorMessage(gz.ErrorUnauthorized)

	// test system admin role

	// create test group and resource
	_, em := globals.Permissions.AddUserGroupRole("owner3", "group3", permissions.Owner)
	assert.Nil(t, em)
	_, err := globals.Permissions.AddPermission("owner3", "resource3", permissions.Read)
	assert.NoError(t, err)

	// system admin should have full permission
	sysAdminPermissionsTestsData := []userResourcePermissionsTest{
		{"sys admin can read group", sysAdminForTest, "group3", permissions.Read, true, nil},
		{"sys admin can write group", sysAdminForTest, "group3", permissions.Write, true, nil},
		{"sys admin can read resource", sysAdminForTest, "resource3", permissions.Read, true, nil},
		{"sys admin can write resource", sysAdminForTest, "resource3", permissions.Write, true, nil},
	}
	testUserResourcePermissions(t, sysAdminPermissionsTestsData)

	// first check user2 does not have access
	user2DoesntHavePermissionsTestsData := []userResourcePermissionsTest{
		{"user2 cannot read group", "user2", "group3", permissions.Read, false, unauth},
		{"user2 cannot write group", "user2", "group3", permissions.Write, false, unauth},
		{"user2 cannot read resource", "user2", "resource3", permissions.Read, false, unauth},
		{"user2 cannot write resource", "user2", "resource3", permissions.Write, false, unauth},
	}
	testUserResourcePermissions(t, user2DoesntHavePermissionsTestsData)

	otherSA := "user2"
	assert.NoError(t, globals.Permissions.Reload(otherSA))

	oldSysAdminPermissionsTestsData := []userResourcePermissionsTest{
		{"old sys admin cannot read group", sysAdminForTest, "group3", permissions.Read, false, unauth},
		{"old sys admin cannot write group", sysAdminForTest, "group3", permissions.Write, false, unauth},
		{"old sys admin cannot read resource", sysAdminForTest, "resource3", permissions.Read, false, unauth},
		{"old sys admin cannot write resource", sysAdminForTest, "resource3", permissions.Write, false, unauth},
	}
	testUserResourcePermissions(t, oldSysAdminPermissionsTestsData)

	newSysAdminPermissionsTestsData := []userResourcePermissionsTest{
		{"new sys admin can read group", "user2", "group3", permissions.Read, true, nil},
		{"new sys admin can write group", "user2", "group3", permissions.Write, true, nil},
		{"new sys admin can read resource", "user2", "resource3", permissions.Read, true, nil},
		{"new sys admin can write resource", "user2", "resource3", permissions.Write, true, nil},
	}
	testUserResourcePermissions(t, newSysAdminPermissionsTestsData)

	// Test with multiple system admins
	assert.NoError(t, globals.Permissions.Reload("user3, user2,    "))
	// user2 should still be the system admin
	testUserResourcePermissions(t, newSysAdminPermissionsTestsData)
	// and also user3 should be a sysadmin
	newSysAdminPermissionsTestsData = []userResourcePermissionsTest{
		{"user3 sys admin can read group", "user3", "group3", permissions.Read, true, nil},
		{"user3 sys admin can write group", "user3", "group3", permissions.Write, true, nil},
		{"user3 sys admin can read resource", "user3", "resource3", permissions.Read, true, nil},
		{"user3 sys admin can write resource", "user3", "resource3", permissions.Write, true, nil},
	}
	testUserResourcePermissions(t, newSysAdminPermissionsTestsData)
}

// TestUserResourcePermissions test user read/write permissions on a resource
func TestUserResourcePermissions(t *testing.T) {

	setup()

	unauthorizedErrMsg := gz.NewErrorMessage(gz.ErrorUnauthorized)

	// test add read permission
	_, err := globals.Permissions.AddPermission("user1", "resource1", permissions.Read)
	assert.NoError(t, err)

	readPermissionsTestsData := []userResourcePermissionsTest{
		{"user can read", "user1", "resource1", permissions.Read, true, nil},
		{"user can't write", "user1", "resource1", permissions.Write, false, unauthorizedErrMsg},
		{"other user can't read", "user2", "resource1", permissions.Read, false, unauthorizedErrMsg},
		{"no resource", "user1", "resource2", permissions.Read, false, unauthorizedErrMsg},
	}
	testUserResourcePermissions(t, readPermissionsTestsData)

	// test add read and write permissions
	_, err = globals.Permissions.AddPermission("user2", "resource2", permissions.Read)
	assert.NoError(t, err)
	_, err = globals.Permissions.AddPermission("user2", "resource2", permissions.Write)
	assert.NoError(t, err)

	writePermissionsTestsData := []userResourcePermissionsTest{
		{"user can read", "user2", "resource2", permissions.Read, true, nil},
		{"user can write", "user2", "resource2", permissions.Write, true, nil},
		{"other user can't read", "user1", "resource2", permissions.Read, false, unauthorizedErrMsg},
		{"other user can't write", "user1", "resource2", permissions.Write, false, unauthorizedErrMsg},
	}
	testUserResourcePermissions(t, writePermissionsTestsData)

	// test remove read permission
	_, err = globals.Permissions.RemovePermission("user1", "resource1", permissions.Read)
	assert.NoError(t, err)

	removeReadPermissionsTestsData := []userResourcePermissionsTest{
		{"user can't read", "user1", "resource1", permissions.Read, false, unauthorizedErrMsg},
		{"user can't write", "user1", "resource1", permissions.Write, false, unauthorizedErrMsg},
		{"other user can't read", "user2", "resource1", permissions.Read, false, unauthorizedErrMsg},
		{"other user can't write", "user2", "resource1", permissions.Write, false, unauthorizedErrMsg},
	}
	testUserResourcePermissions(t, removeReadPermissionsTestsData)

	// test remove write permission
	_, err = globals.Permissions.RemovePermission("user2", "resource2", permissions.Write)
	assert.NoError(t, err)

	removeWritePermissionsTestsData := []userResourcePermissionsTest{
		{"user can read", "user2", "resource2", permissions.Read, true, nil},
		{"user can't write", "user2", "resource2", permissions.Write, false, unauthorizedErrMsg},
		{"other user can't read", "user1", "resource2", permissions.Read, false, unauthorizedErrMsg},
		{"other user can't write", "user1", "resource2", permissions.Write, false, unauthorizedErrMsg},
	}
	testUserResourcePermissions(t, removeWritePermissionsTestsData)

	// test remove write permission when user has read only permission
	// this should have no effect
	_, err = globals.Permissions.AddPermission("user3", "resource3", permissions.Read)
	assert.NoError(t, err)
	_, err = globals.Permissions.RemovePermission("user3", "resource3", permissions.Write)
	assert.NoError(t, err)

	removeWriteOnReadPermissionsTestsData := []userResourcePermissionsTest{
		{"user can read", "user3", "resource3", permissions.Read, true, nil},
		{"user can't write", "user3", "resource3", permissions.Write, false, unauthorizedErrMsg},
	}
	testUserResourcePermissions(t, removeWriteOnReadPermissionsTestsData)

	// test remove read permission when user has read and write permission
	_, err = globals.Permissions.AddPermission("user4", "resource4", permissions.Write)
	assert.NoError(t, err)
	_, err = globals.Permissions.RemovePermission("user4", "resource4", permissions.Read)
	assert.NoError(t, err)

	removeReadOnWritePermissionsTestsData := []userResourcePermissionsTest{
		{"user can't read", "user4", "resource4", permissions.Read, false, unauthorizedErrMsg},
		{"user can write", "user4", "resource4", permissions.Write, true, nil},
	}
	testUserResourcePermissions(t, removeReadOnWritePermissionsTestsData)

	// the next two test are connected
	// Test remove resource and verify all associated permissions are also be
	// removed.
	// first add user permissions to the same resource and verify
	_, err = globals.Permissions.AddPermission("userA", "resourceA", permissions.Read)
	assert.NoError(t, err)
	_, err = globals.Permissions.AddPermission("userB", "resourceA", permissions.Write)
	assert.NoError(t, err)
	_, err = globals.Permissions.AddPermission("userC", "resourceA", permissions.Read)
	assert.NoError(t, err)
	_, err = globals.Permissions.AddPermission("userC", "resourceA", permissions.Write)
	assert.NoError(t, err)

	addResourcePermissionsTestsData := []userResourcePermissionsTest{
		{"userA can read", "userA", "resourceA", permissions.Read, true, nil},
		{"userB can write", "userB", "resourceA", permissions.Write, true, nil},
		{"userC can read", "userC", "resourceA", permissions.Read, true, nil},
		{"userC can write", "userC", "resourceA", permissions.Write, true, nil},
	}
	testUserResourcePermissions(t, addResourcePermissionsTestsData)

	// now remove the resource
	_, em := globals.Permissions.RemoveResource("resourceA")
	assert.NoError(t, em.BaseError)
	removeResourcePermissionsTestsData := []userResourcePermissionsTest{
		{"userA can't read", "userA", "resourceA", permissions.Read, false, unauthorizedErrMsg},
		{"userB can't write", "userA", "resourceA", permissions.Write, false, unauthorizedErrMsg},
		{"userC can't read", "userC", "resourceA", permissions.Read, false, unauthorizedErrMsg},
		{"userC can't write", "userC", "resourceA", permissions.Write, false, unauthorizedErrMsg},
	}
	testUserResourcePermissions(t, removeResourcePermissionsTestsData)
}

// TestUserRolePermissions test role permissions and permission inheritance
// for users in a group
func TestUserRolePermissions(t *testing.T) {

	setup()

	unauthorizedErrMsg := gz.NewErrorMessage(gz.ErrorUnauthorized)

	// test basic role read/write permissions
	_, em := globals.Permissions.AddUserGroupRole("ownerA", "groupA", permissions.Owner)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("adminA", "groupA", permissions.Admin)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("memberA", "groupA", permissions.Member)
	assert.NoError(t, em.BaseError)

	rolePermissionsTestsData := []userResourcePermissionsTest{
		{"owner can read", "ownerA", "groupA", permissions.Read, true, nil},
		{"owner can write", "ownerA", "groupA", permissions.Write, true, nil},
		{"admin can read", "adminA", "groupA", permissions.Read, true, nil},
		{"admin can write", "adminA", "groupA", permissions.Write, true, nil},
		{"member can read", "memberA", "groupA", permissions.Read, true, nil},
		{"member can't write", "memberA", "groupA", permissions.Write, false, unauthorizedErrMsg},
		{"external can't read", "external", "groupA", permissions.Read, false, unauthorizedErrMsg},
		{"external can't write", "external", "groupA", permissions.Write, false, unauthorizedErrMsg},
	}
	testUserResourcePermissions(t, rolePermissionsTestsData)

	// test role read permission
	_, em = globals.Permissions.AddUserGroupRole("owner1", "group1", permissions.Owner)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("admin1", "group1", permissions.Admin)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("member1", "group1", permissions.Member)
	assert.NoError(t, em.BaseError)
	_, err := globals.Permissions.AddPermission("group1", "resource1", permissions.Read)
	assert.NoError(t, err)

	readPermissionsTestsData := []userResourcePermissionsTest{
		{"group can read", "group1", "resource1", permissions.Read, true, nil},
		{"owner can read", "owner1", "resource1", permissions.Read, true, nil},
		{"owner can't write", "owner1", "resource1", permissions.Write, false, unauthorizedErrMsg},
		{"admin can read", "admin1", "resource1", permissions.Read, true, nil},
		{"admin can't write", "admin1", "resource1", permissions.Write, false, unauthorizedErrMsg},
		{"member can read", "member1", "resource1", permissions.Read, true, nil},
		{"member can't write", "member1", "resource1", permissions.Write, false, unauthorizedErrMsg},
		{"external can't read", "external", "resource1", permissions.Read, false, unauthorizedErrMsg},
		{"external can't write", "external", "resource1", permissions.Write, false, unauthorizedErrMsg},
	}
	testUserResourcePermissions(t, readPermissionsTestsData)

	// test role write permission
	_, em = globals.Permissions.AddUserGroupRole("owner2", "group2", permissions.Owner)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("admin2", "group2", permissions.Admin)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("member2", "group2", permissions.Member)
	assert.NoError(t, em.BaseError)
	_, err = globals.Permissions.AddPermission("group2", "resource2", permissions.Write)
	assert.NoError(t, err)

	writePermissionsTestsData := []userResourcePermissionsTest{
		{"group can write", "group2", "resource2", permissions.Write, true, nil},
		{"owner can write", "owner2", "resource2", permissions.Write, true, nil},
		{"admin can write", "admin2", "resource2", permissions.Write, true, nil},
		{"member can write", "member2", "resource2", permissions.Write, true, nil},
		{"external can't write", "external", "resource2", permissions.Write, false, unauthorizedErrMsg},
	}
	testUserResourcePermissions(t, writePermissionsTestsData)

	// test system admin role
	// system admin should have full permission

	// create test group and resource
	_, em = globals.Permissions.AddUserGroupRole("owner3", "group3", permissions.Owner)
	assert.NoError(t, em.BaseError)
	_, err = globals.Permissions.AddPermission("owner3", "resource3", permissions.Read)
	assert.NoError(t, err)

	// NOTE: 'rootfortests' is the built-in system administrator username used only in tests
	sysAdminPermissionsTestsData := []userResourcePermissionsTest{
		{"sys admin can read group", sysAdminForTest, "group3", permissions.Read, true, nil},
		{"sys admin can write group", sysAdminForTest, "group3", permissions.Write, true, nil},
		{"sys admin can read resource", sysAdminForTest, "resource3", permissions.Read, true, nil},
		{"sys admin can write resource", sysAdminForTest, "resource3", permissions.Write, true, nil},
	}
	testUserResourcePermissions(t, sysAdminPermissionsTestsData)
}

// testUserResourcePermissions checks if a user is authorized to perform an
// action on a resource. If not, it checks if the correct error code is
// returned.
func testUserResourcePermissions(t *testing.T, data []userResourcePermissionsTest) {
	for _, test := range data {
		t.Run(test.testDesc, func(t *testing.T) {
			ok, em := globals.Permissions.IsAuthorized(test.user, test.resource, test.action)
			assert.Equal(t, test.expAuthorized, ok)
			if test.expErrMsg != nil {
				assert.Equal(t, test.expErrMsg.ErrCode, em.ErrCode)
				assert.Equal(t, test.expErrMsg.StatusCode, em.StatusCode)
			} else {
				assert.Nil(t, em)
			}
		})
	}
}

type userGroupsTest struct {
	// description of the test
	testDesc string
	// username
	user string
	// expected groups and roles
	expGroups map[string]string
}

// TestGetGroupsAndRolesForUser test returning the groups of an user.
func TestGetGroupsAndRolesForUser(t *testing.T) {
	// test basic role read/write permissions
	_, em := globals.Permissions.AddUserGroupRole("userA", "groupA", permissions.Owner)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("userA", "groupA", permissions.Admin)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("userA", "groupA", permissions.Member)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("userA", "group2", permissions.Admin)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("userB", "groupA", permissions.Member)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("userB", "group2", permissions.Owner)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("userC", "group2", permissions.Member)
	assert.NoError(t, em.BaseError)
	_, em = globals.Permissions.AddUserGroupRole("userU", "group_with-underscore_", permissions.Member)
	assert.NoError(t, em.BaseError)

	userGroupsTestData := []userGroupsTest{
		{testDesc: "groups of userA", user: "userA", expGroups: map[string]string{"groupA": "owner", "group2": "admin"}},
		{testDesc: "groups of userB", user: "userB", expGroups: map[string]string{"groupA": "member", "group2": "owner"}},
		{testDesc: "groups of userC", user: "userC", expGroups: map[string]string{"group2": "member"}},
		{testDesc: "groups of unexistent userD", user: "userD", expGroups: map[string]string{}},
		{testDesc: "group name with underscore", user: "userU", expGroups: map[string]string{"group_with-underscore_": "member"}},
	}
	testGetUserGroups(t, userGroupsTestData)
}

func testGetUserGroups(t *testing.T, data []userGroupsTest) {
	for _, test := range data {
		t.Run(test.testDesc, func(t *testing.T) {
			groups := globals.Permissions.GetGroupsAndRolesForUser(test.user)
			if test.expGroups != nil {
				assert.Len(t, groups, len(test.expGroups))
				for g, role := range test.expGroups {
					assert.Contains(t, groups, g)
					assert.Equal(t, role, groups[g])
				}
			} else {
				assert.Empty(t, groups)
			}
		})
	}
}
