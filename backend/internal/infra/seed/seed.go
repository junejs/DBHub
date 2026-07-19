// Package seed installs the v1 builtin rows on first boot (and re-runs as a no-op
// on subsequent boots). Source of truth: docs/prd/10-data-model.md §4 (environments,
// environment_policies, roles) and docs/prd/02-permission-and-access.md §2
// (builtin role list). The role_permissions rows come from the code-side registry
// in package permissions.
//
// Idempotency: every INSERT uses ON CONFLICT DO NOTHING keyed on the natural key
// (environments.key, roles.key). For role_permissions we delete-insert per role id:
// the table is small, fully owned by the registry, and there is no production data
// to disturb. Re-running SeedBuiltin is safe; row counts are stable.
package seed

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/junepy/dbhub/backend/internal/infra/permissions"
)

// EnvironmentRow is the bun-mapped view of the `environments` table for the seed.
// We keep it local to this package; the canonical model lives elsewhere.
// The explicit `bun:"table:environments"` opt out of bun's pluralized default
// inflector (which would otherwise derive "environment_rows").
type EnvironmentRow struct {
	bun.BaseModel `bun:"table:environments"`
	Key           string `bun:"key,pk"`
	Name          string `bun:"name"`
	ProtectionLvl int    `bun:"protection_level"`
	Color         string `bun:"color"`
	Rank          int    `bun:"rank"`
}

// BuiltinEnvironments is the v1 set per PRD §4. prod > stage > test > dev by
// protection_level and rank (larger = stricter / later in UI order).
var BuiltinEnvironments = []EnvironmentRow{
	{Key: "prod", Name: "Production", ProtectionLvl: 40, Color: "#d4351c", Rank: 40},
	{Key: "stage", Name: "Staging", ProtectionLvl: 30, Color: "#f47738", Rank: 30},
	{Key: "test", Name: "Test", ProtectionLvl: 20, Color: "#1d70b8", Rank: 20},
	{Key: "dev", Name: "Development", ProtectionLvl: 10, Color: "#00703c", Rank: 10},
}

// EnvironmentPolicyRow maps to the `environment_policies` table.
type EnvironmentPolicyRow struct {
	bun.BaseModel `bun:"table:environment_policies"`
	EnvironmentID int64 `bun:"environment_id,pk"`
	QueryRowLimit int   `bun:"query_row_limit"`
	ExportMaxRows int64 `bun:"export_max_rows"`
}

// BuiltinEnvironmentPolicies matches PRD §4. prod is strictest (1k / 100k),
// dev/test are most permissive (10k / 1M).
var BuiltinEnvironmentPolicies = []EnvironmentPolicyRow{
	{QueryRowLimit: 1000, ExportMaxRows: 100000}, // env_id filled at seed time
	{QueryRowLimit: 5000, ExportMaxRows: 500000},
	{QueryRowLimit: 10000, ExportMaxRows: 1000000},
	{QueryRowLimit: 10000, ExportMaxRows: 1000000},
}

// RoleRow maps to the `roles` table.
type RoleRow struct {
	bun.BaseModel `bun:"table:roles"`
	Key           string `bun:"key,pk"`
	Name          string `bun:"name"`
	Scope         string `bun:"scope"`
	Builtin       bool   `bun:"builtin"`
}

// BuiltinRoles mirrors PRD §4 — exactly 8 rows.
var BuiltinRoles = []RoleRow{
	{Key: permissions.RoleWorkspaceAdmin, Name: "Workspace Admin", Scope: "workspace", Builtin: true},
	{Key: permissions.RoleSecurityAdmin, Name: "Security Admin", Scope: "workspace", Builtin: true},
	{Key: permissions.RoleWorkspaceMember, Name: "Workspace Member", Scope: "workspace", Builtin: true},
	{Key: permissions.RoleProjectOwner, Name: "Project Owner", Scope: "project", Builtin: true},
	{Key: permissions.RoleProjectDBA, Name: "Project DBA", Scope: "project", Builtin: true},
	{Key: permissions.RoleSQLEditorUser, Name: "SQL Editor User", Scope: "project", Builtin: true},
	{Key: permissions.RoleSQLEditorReadUser, Name: "SQL Editor Read User", Scope: "project", Builtin: true},
	{Key: permissions.RoleProjectViewer, Name: "Project Viewer", Scope: "project", Builtin: true},
}

// SeedBuiltin applies the v1 builtin rows (environments, environment_policies,
// roles, role_permissions) to the platform database. Safe to call repeatedly.
// Returns nil on success even if every row was already present.
func SeedBuiltin(ctx context.Context, db *bun.DB) error {
	if db == nil {
		return fmt.Errorf("seed.SeedBuiltin: nil bun.DB")
	}
	if err := seedEnvironments(ctx, db); err != nil {
		return err
	}
	if err := seedEnvironmentPolicies(ctx, db); err != nil {
		return err
	}
	if err := seedRoles(ctx, db); err != nil {
		return err
	}
	if err := seedRolePermissions(ctx, db); err != nil {
		return err
	}
	return nil
}

func seedEnvironments(ctx context.Context, db *bun.DB) error {
	for _, env := range BuiltinEnvironments {
		_, err := db.NewInsert().
			Model(&env).
			On("CONFLICT (key) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("seed environments[%s]: %w", env.Key, err)
		}
	}
	return nil
}

func seedEnvironmentPolicies(ctx context.Context, db *bun.DB) error {
	for i, pol := range BuiltinEnvironmentPolicies {
		envKey := BuiltinEnvironments[i].Key
		// Resolve environment_id from environments.key.
		var envID int64
		if err := db.NewSelect().
			TableExpr("environments").
			Column("id").
			Where("key = ?", envKey).
			Scan(ctx, &envID); err != nil {
			return fmt.Errorf("seed environment_policies[%s]: lookup env id: %w", envKey, err)
		}
		if envID == 0 {
			return fmt.Errorf("seed environment_policies[%s]: environment not found", envKey)
		}
		pol.EnvironmentID = envID
		_, err := db.NewInsert().
			Model(&pol).
			On("CONFLICT (environment_id) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("seed environment_policies[%s]: %w", envKey, err)
		}
	}
	return nil
}

func seedRoles(ctx context.Context, db *bun.DB) error {
	for _, role := range BuiltinRoles {
		_, err := db.NewInsert().
			Model(&role).
			On("CONFLICT (key) DO NOTHING").
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("seed roles[%s]: %w", role.Key, err)
		}
	}
	return nil
}

// seedRolePermissions delete-inserts role_permissions per builtin role id.
// The set is small and fully owned by the code-side registry, so wiping the
// role's rows and re-inserting the current registry keeps the DB consistent
// with the code. roles.key is the natural identifier.
func seedRolePermissions(ctx context.Context, db *bun.DB) error {
	for _, roleKey := range builtinRoleKeys() {
		var roleID int64
		if err := db.NewSelect().
			TableExpr("roles").
			Column("id").
			Where("key = ?", roleKey).
			Scan(ctx, &roleID); err != nil {
			return fmt.Errorf("seed role_permissions[%s]: lookup role id: %w", roleKey, err)
		}
		if roleID == 0 {
			return fmt.Errorf("seed role_permissions[%s]: role not found", roleKey)
		}

		if _, err := db.NewDelete().
			TableExpr("role_permissions").
			Where("role_id = ?", roleID).
			Exec(ctx); err != nil {
			return fmt.Errorf("seed role_permissions[%s]: clear existing: %w", roleKey, err)
		}

		perms := permissions.Registry[roleKey]
		if len(perms) == 0 {
			// Empty role (e.g. sqlEditorUser is a v2 placeholder) — leave row count at 0.
			continue
		}
		rows := make([]rolePermissionRow, 0, len(perms))
		for _, p := range perms {
			rows = append(rows, rolePermissionRow{RoleID: roleID, Permission: p})
		}
		if _, err := db.NewInsert().Model(&rows).Exec(ctx); err != nil {
			return fmt.Errorf("seed role_permissions[%s]: insert: %w", roleKey, err)
		}
	}
	return nil
}

// rolePermissionRow matches role_permissions column shape; local to this package.
type rolePermissionRow struct {
	bun.BaseModel `bun:"table:role_permissions"`
	RoleID        int64  `bun:"role_id"`
	Permission    string `bun:"permission"`
}

func builtinRoleKeys() []string {
	keys := make([]string, 0, len(BuiltinRoles))
	for _, r := range BuiltinRoles {
		keys = append(keys, r.Key)
	}
	return keys
}
