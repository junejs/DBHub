// Package projectsqlc holds lightweight scan-target structs for the `projects`
// table. We deliberately keep these here (separate from service.Project) so the
// domain type stays decoupled from bun and free of ORM tags.
package projectsqlc

// Project mirrors the columns we read from the `projects` table for list
// operations. Description is intentionally omitted — the v1 list response uses
// the project title (Name) only; description is fetched on Get (later issue).
type Project struct {
	Key  string `bun:"key"`
	Name string `bun:"name"`
}
