package models

import "gorm.io/gorm"

// purgeRolePermissions deletes sys_role_permissions rows that match
// scope, in a session isolated from the caller's transaction: NewDB
// detaches the connection so admin's tenant callbacks don't re-trigger,
// and SkipHooks avoids recursing into GORM hooks we may have installed.
//
// Centralising the NewDB/SkipHooks dance means future cross-model
// cascades (e.g. Department.AfterDelete) can opt in by adding a
// one-liner with a scope closure, instead of re-implementing the same
// session recipe and risking a partial copy.
func purgeRolePermissions(tx *gorm.DB, scope func(*gorm.DB) *gorm.DB) error {
	return scope(tx.Session(&gorm.Session{NewDB: true, SkipHooks: true})).
		Delete(&RolePermission{}).Error
}
