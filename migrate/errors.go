package migrate

import "errors"

var (
	ErrMigrationIDRequired    = errors.New("migration id is required")
	ErrMigrationExists        = errors.New("migration already exists")
	ErrMigrationUpRequired    = errors.New("migration up function or sql is required")
	ErrMigrationNotFound      = errors.New("migration not found")
	ErrMigrationNotApplied    = errors.New("migration not applied")
	ErrLockAcquireFailed      = errors.New("failed to acquire migration lock")
	ErrNoMigrations           = errors.New("no migrations registered")
	ErrInvalidVersion         = errors.New("invalid version format")
	ErrMigrationFailed        = errors.New("migration failed")
	ErrRollbackFailed         = errors.New("rollback failed")
)
