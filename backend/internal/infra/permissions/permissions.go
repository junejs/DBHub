// Package permissions is the single source of truth for v1 permission strings
// (PRD 02 §2.2 naming: `db.<resource>.<verb>`). Both the seed step
// (role_permissions table) and the future IAM engine read from this registry.
// Adding a new permission = add a constant here + assign it to a role in Registry.
package permissions

// Permission is a flat string identifier (e.g. "db.sql.select").
type Permission = string

// Workspace-scoped permission constants. Workspace-level operations that cut
// across projects (admin, IdP, audit browse, environment policy management).
const (
	// Projects
	PermProjectsList   Permission = "db.projects.list"
	PermProjectsGet    Permission = "db.projects.get"
	PermProjectsCreate Permission = "db.projects.create"
	PermProjectsUpdate Permission = "db.projects.update"
	PermProjectsDelete Permission = "db.projects.delete"
	PermProjectsGetIAM Permission = "db.projects.getIamPolicy"
	PermProjectsSetIAM Permission = "db.projects.setIamPolicy"

	// Environments & policies
	PermEnvironmentsList    Permission = "db.environments.list"
	PermEnvironmentsUpdate  Permission = "db.environments.update"
	PermEnvironmentPolicies Permission = "db.environmentPolicies.update"

	// Identity & IdP
	PermUsersList    Permission = "db.users.list"
	PermUsersGet     Permission = "db.users.get"
	PermUsersCreate  Permission = "db.users.create"
	PermUsersUpdate  Permission = "db.users.update"
	PermUsersDelete  Permission = "db.users.delete"
	PermGroupsList   Permission = "db.groups.list"
	PermGroupsUpdate Permission = "db.groups.update"
	PermIDPList      Permission = "db.identityProviders.list"
	PermIDPUpdate    Permission = "db.identityProviders.update"

	// Audit
	PermAuditLogsSearch Permission = "db.auditLogs.search"
	PermAuditLogsExport Permission = "db.auditLogs.export"

	// Settings
	PermSettingsRead   Permission = "db.settings.read"
	PermSettingsUpdate Permission = "db.settings.update"
)

// Project-scoped permission constants. Project resources (instances, databases,
// query history, worksheets, favorites) use these; the ACL layer additionally
// requires the caller to have a role binding on the target project.
const (
	// Instances & data sources (project governance)
	PermInstancesList   Permission = "db.instances.list"
	PermInstancesGet    Permission = "db.instances.get"
	PermInstancesCreate Permission = "db.instances.create"
	PermInstancesUpdate Permission = "db.instances.update"
	PermInstancesDelete Permission = "db.instances.delete"
	PermInstancesSync   Permission = "db.instances.sync"

	// Databases
	PermDatabasesList   Permission = "db.databases.list"
	PermDatabasesGet    Permission = "db.databases.get"
	PermDatabasesSync   Permission = "db.databases.sync"
	PermDatabasesUpdate Permission = "db.databases.update"

	// SQL execution
	PermSQLSelect  Permission = "db.sql.select"
	PermSQLExplain Permission = "db.sql.explain"
	PermSQLInfo    Permission = "db.sql.info"
	// PermSQLDML / PermSQLDDL are intentionally absent: v1 ships read-only path.
	// DML/DDL belong to change management (v2 per PRD 02 §2.1 footnote).

	// Worksheets / favorites
	PermWorksheetsList   Permission = "db.worksheets.list"
	PermWorksheetsCreate Permission = "db.worksheets.create"
	PermWorksheetsUpdate Permission = "db.worksheets.update"
	PermWorksheetsDelete Permission = "db.worksheets.delete"
	PermFavoritesList    Permission = "db.favorites.list"
	PermFavoritesCreate  Permission = "db.favorites.create"
	PermFavoritesDelete  Permission = "db.favorites.delete"

	// Exports
	PermExportsCreate Permission = "db.exports.create"

	// Query history (self)
	PermQueryHistoryList Permission = "db.queryHistory.list"
)

// RoleKey names match PRD 10-data-model §4 / PRD 02 §2.1 and the `roles.key` column.
type RoleKey = string

const (
	RoleWorkspaceAdmin    RoleKey = "workspaceAdmin"
	RoleSecurityAdmin     RoleKey = "securityAdmin"
	RoleWorkspaceMember   RoleKey = "workspaceMember"
	RoleProjectOwner      RoleKey = "projectOwner"
	RoleProjectDBA        RoleKey = "projectDBA"
	RoleSQLEditorUser     RoleKey = "sqlEditorUser"
	RoleSQLEditorReadUser RoleKey = "sqlEditorReadUser"
	RoleProjectViewer     RoleKey = "projectViewer"
)

// Registry is the code-side mapping of builtin role -> granted permissions.
// Authoritative for v1; custom roles land in a later issue (PRD 02 §2.1 footnote
// says v1 ships builtin roles only). Each list is the union of permissions that
// role grants, evaluated by the future IAM engine.
var Registry = map[RoleKey][]Permission{
	RoleWorkspaceAdmin: {
		// Platform-wide governance: every workspace-scoped capability + project admin view.
		PermProjectsList, PermProjectsGet, PermProjectsCreate, PermProjectsUpdate, PermProjectsDelete,
		PermProjectsGetIAM, PermProjectsSetIAM,
		PermEnvironmentsList, PermEnvironmentsUpdate, PermEnvironmentPolicies,
		PermUsersList, PermUsersGet, PermUsersCreate, PermUsersUpdate, PermUsersDelete,
		PermGroupsList, PermGroupsUpdate,
		PermIDPList, PermIDPUpdate,
		PermAuditLogsSearch, PermAuditLogsExport,
		PermSettingsRead, PermSettingsUpdate,
	},
	RoleSecurityAdmin: {
		// Audit + environment policy oversight. No connection credentials, no project IAM edits.
		PermProjectsList, PermProjectsGet,
		PermEnvironmentsList, PermEnvironmentsUpdate, PermEnvironmentPolicies,
		PermUsersList, PermUsersGet,
		PermGroupsList,
		PermIDPList,
		PermAuditLogsSearch, PermAuditLogsExport,
		PermSettingsRead,
	},
	RoleWorkspaceMember: {
		// Browse own history, read settings. Cannot list all projects.
		PermProjectsList, PermProjectsGet,
		PermUsersGet,
		PermQueryHistoryList,
		PermSettingsRead,
	},
	RoleProjectOwner: {
		// Full project governance: IAM + every project-scoped capability + read project audit.
		PermProjectsGet, PermProjectsGetIAM, PermProjectsSetIAM,
		PermInstancesList, PermInstancesGet, PermInstancesCreate, PermInstancesUpdate, PermInstancesDelete, PermInstancesSync,
		PermDatabasesList, PermDatabasesGet, PermDatabasesSync, PermDatabasesUpdate,
		PermSQLSelect, PermSQLExplain, PermSQLInfo,
		PermWorksheetsList, PermWorksheetsCreate, PermWorksheetsUpdate, PermWorksheetsDelete,
		PermFavoritesList, PermFavoritesCreate, PermFavoritesDelete,
		PermExportsCreate,
		PermQueryHistoryList,
		PermAuditLogsSearch,
	},
	RoleProjectDBA: {
		// Instance / database / schema management. NO project IAM editing.
		PermProjectsGet,
		PermInstancesList, PermInstancesGet, PermInstancesCreate, PermInstancesUpdate, PermInstancesDelete, PermInstancesSync,
		PermDatabasesList, PermDatabasesGet, PermDatabasesSync, PermDatabasesUpdate,
		PermSQLSelect, PermSQLExplain, PermSQLInfo,
		PermWorksheetsList, PermWorksheetsCreate, PermWorksheetsUpdate, PermWorksheetsDelete,
		PermFavoritesList, PermFavoritesCreate, PermFavoritesDelete,
		PermExportsCreate,
		PermQueryHistoryList,
	},
	RoleSQLEditorUser: {
		// Read/write SQL editor — placeholder per PRD 02 §2.1: not granted in v1.
		// Listed so the role exists in the registry but seeding it with no v1 grant is the
		// engine's call (we still seed empty role_permissions row to keep the registry contract).
	},
	RoleSQLEditorReadUser: {
		PermProjectsGet,
		PermDatabasesList, PermDatabasesGet,
		PermSQLSelect, PermSQLExplain, PermSQLInfo,
		PermWorksheetsList, PermWorksheetsCreate, PermWorksheetsUpdate, PermWorksheetsDelete,
		PermFavoritesList, PermFavoritesCreate, PermFavoritesDelete,
		PermQueryHistoryList,
	},
	RoleProjectViewer: {
		// Browse schema only. No data queries, no SQL editor.
		PermProjectsGet,
		PermInstancesList, PermInstancesGet,
		PermDatabasesList, PermDatabasesGet,
		PermWorksheetsList,
		PermFavoritesList, PermFavoritesCreate, PermFavoritesDelete,
	},
}

// All returns the unique set of permissions defined across the registry. Useful
// for sanity tests and to ensure role_permissions table has the right rows.
func All() []Permission {
	seen := make(map[Permission]struct{})
	for _, perms := range Registry {
		for _, p := range perms {
			seen[p] = struct{}{}
		}
	}
	out := make([]Permission, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}
