// Package database provides all SQLite persistence for Kestrel.
// It opens the database in WAL mode, runs schema migrations, and exposes
// typed CRUD helpers for every domain (assets, vulnerabilities, projects,
// conversations, HITL, audit, RBAC, C2, workflows, and more).
// All tests in this package require CGO_ENABLED=1 (go-sqlite3 dependency).
package database
