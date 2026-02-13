package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Direction string

const (
	Up   Direction = "up"
	Down Direction = "down"
)

type Migration struct {
	ID          string
	Name        string
	Description string
	Version     string
	UpSQL       string
	DownSQL     string
	UpFunc      func(ctx context.Context, db *sql.DB) error
	DownFunc    func(ctx context.Context, db *sql.DB) error
	CreatedAt   time.Time
}

type MigrationRecord struct {
	ID        string
	Version   string
	AppliedAt time.Time
	Duration  time.Duration
	Status    string
	Error     string
}

type Config struct {
	TableName      string
	LockTableName  string
	LockTimeout    time.Duration
	RetryCount     int
	RetryDelay     time.Duration
	ValidateOnMig  bool
}

type Migrator struct {
	db        *sql.DB
	config    Config
	migrations map[string]*Migration
	mu        sync.RWMutex
}

func NewMigrator(db *sql.DB, config Config) *Migrator {
	if config.TableName == "" {
		config.TableName = "schema_migrations"
	}
	if config.LockTableName == "" {
		config.LockTableName = "schema_migrations_lock"
	}
	if config.LockTimeout == 0 {
		config.LockTimeout = time.Minute * 5
	}
	if config.RetryCount == 0 {
		config.RetryCount = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = time.Second
	}

	return &Migrator{
		db:         db,
		config:     config,
		migrations: make(map[string]*Migration),
	}
}

func (m *Migrator) Register(migration *Migration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if migration.ID == "" {
		return ErrMigrationIDRequired
	}

	if _, exists := m.migrations[migration.ID]; exists {
		return fmt.Errorf("%w: %s", ErrMigrationExists, migration.ID)
	}

	if migration.UpSQL == "" && migration.UpFunc == nil {
		return ErrMigrationUpRequired
	}

	m.migrations[migration.ID] = migration
	return nil
}

func (m *Migrator) RegisterBatch(migrations []*Migration) error {
	for _, migration := range migrations {
		if err := m.Register(migration); err != nil {
			return err
		}
	}
	return nil
}

func (m *Migrator) Init(ctx context.Context) error {
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(255) PRIMARY KEY,
			version VARCHAR(255) NOT NULL,
			applied_at TIMESTAMP NOT NULL,
			duration_ms BIGINT,
			status VARCHAR(50) NOT NULL,
			error TEXT
		);
	`, m.config.TableName)

	if _, err := m.db.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	createLockTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INT PRIMARY KEY DEFAULT 1,
			locked_at TIMESTAMP,
			locked_by VARCHAR(255),
			CHECK (id = 1)
		);
	`, m.config.LockTableName)

	if _, err := m.db.ExecContext(ctx, createLockTableSQL); err != nil {
		return fmt.Errorf("create lock table: %w", err)
	}

	return nil
}

func (m *Migrator) Up(ctx context.Context) error {
	if err := m.acquireLock(ctx); err != nil {
		return err
	}
	defer m.releaseLock(ctx)

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	pending := m.getPendingMigrations(applied)
	if len(pending) == 0 {
		return nil
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Version < pending[j].Version
	})

	for _, migration := range pending {
		if err := m.applyMigration(ctx, migration, Up); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.ID, err)
		}
	}

	return nil
}

func (m *Migrator) Down(ctx context.Context, steps int) error {
	if err := m.acquireLock(ctx); err != nil {
		return err
	}
	defer m.releaseLock(ctx)

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		return nil
	}

	sort.Slice(applied, func(i, j int) bool {
		return applied[i].Version > applied[j].Version
	})

	if steps > len(applied) {
		steps = len(applied)
	}

	for i := 0; i < steps; i++ {
		record := applied[i]
		migration, exists := m.migrations[record.ID]
		if !exists {
			return fmt.Errorf("%w: %s", ErrMigrationNotFound, record.ID)
		}

		if err := m.applyMigration(ctx, migration, Down); err != nil {
			return fmt.Errorf("rollback migration %s: %w", migration.ID, err)
		}
	}

	return nil
}

func (m *Migrator) UpTo(ctx context.Context, version string) error {
	if err := m.acquireLock(ctx); err != nil {
		return err
	}
	defer m.releaseLock(ctx)

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	pending := m.getPendingMigrations(applied)
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Version < pending[j].Version
	})

	for _, migration := range pending {
		if migration.Version > version {
			break
		}

		if err := m.applyMigration(ctx, migration, Up); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.ID, err)
		}
	}

	return nil
}

func (m *Migrator) DownTo(ctx context.Context, version string) error {
	if err := m.acquireLock(ctx); err != nil {
		return err
	}
	defer m.releaseLock(ctx)

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	sort.Slice(applied, func(i, j int) bool {
		return applied[i].Version > applied[j].Version
	})

	for _, record := range applied {
		if record.Version <= version {
			break
		}

		migration, exists := m.migrations[record.ID]
		if !exists {
			return fmt.Errorf("%w: %s", ErrMigrationNotFound, record.ID)
		}

		if err := m.applyMigration(ctx, migration, Down); err != nil {
			return fmt.Errorf("rollback migration %s: %w", migration.ID, err)
		}
	}

	return nil
}

func (m *Migrator) Redo(ctx context.Context) error {
	if err := m.acquireLock(ctx); err != nil {
		return err
	}
	defer m.releaseLock(ctx)

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		return nil
	}

	sort.Slice(applied, func(i, j int) bool {
		return applied[i].Version > applied[j].Version
	})

	lastApplied := applied[0]
	migration, exists := m.migrations[lastApplied.ID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrMigrationNotFound, lastApplied.ID)
	}

	if err := m.applyMigration(ctx, migration, Down); err != nil {
		return fmt.Errorf("rollback migration %s: %w", migration.ID, err)
	}

	if err := m.applyMigration(ctx, migration, Up); err != nil {
		return fmt.Errorf("re-apply migration %s: %w", migration.ID, err)
	}

	return nil
}

func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	appliedMap := make(map[string]*MigrationRecord)
	for _, record := range applied {
		appliedMap[record.ID] = record
	}

	var statuses []MigrationStatus
	for _, migration := range m.migrations {
		status := MigrationStatus{
			ID:          migration.ID,
			Name:        migration.Name,
			Version:     migration.Version,
			Description: migration.Description,
		}

		if record, applied := appliedMap[migration.ID]; applied {
			status.Applied = true
			status.AppliedAt = record.AppliedAt
			status.Duration = record.Duration
			status.Status = record.Status
			status.Error = record.Error
		} else {
			status.Applied = false
			status.Status = "pending"
		}

		statuses = append(statuses, status)
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Version < statuses[j].Version
	})

	return statuses, nil
}

type MigrationStatus struct {
	ID          string
	Name        string
	Version     string
	Description string
	Applied     bool
	AppliedAt   time.Time
	Duration    time.Duration
	Status      string
	Error       string
}

func (m *Migrator) applyMigration(ctx context.Context, migration *Migration, direction Direction) error {
	start := time.Now()

	var err error
	var sqlStr string
	var fn func(ctx context.Context, db *sql.DB) error

	if direction == Up {
		sqlStr = migration.UpSQL
		fn = migration.UpFunc
	} else {
		sqlStr = migration.DownSQL
		fn = migration.DownFunc
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	if fn != nil {
		err = fn(ctx, m.db)
	} else if sqlStr != "" {
		_, err = tx.ExecContext(ctx, sqlStr)
	}

	if err != nil {
		m.recordMigration(ctx, migration, direction, start, "failed", err.Error())
		return err
	}

	if direction == Up {
		_, err = tx.ExecContext(ctx,
			fmt.Sprintf("INSERT INTO %s (id, version, applied_at, duration_ms, status) VALUES (?, ?, ?, ?, ?)", m.config.TableName),
			migration.ID, migration.Version, time.Now(), time.Since(start).Milliseconds(), "applied",
		)
	} else {
		_, err = tx.ExecContext(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE id = ?", m.config.TableName),
			migration.ID,
		)
	}

	if err != nil {
		return fmt.Errorf("update migration record: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (m *Migrator) recordMigration(ctx context.Context, migration *Migration, direction Direction, start time.Time, status, errMsg string) {
	record := MigrationRecord{
		ID:        migration.ID,
		Version:   migration.Version,
		AppliedAt: time.Now(),
		Duration:  time.Since(start),
		Status:    fmt.Sprintf("%s_%s", direction, status),
		Error:     errMsg,
	}

	_, _ = m.db.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO %s (id, version, applied_at, duration_ms, status, error) VALUES (?, ?, ?, ?, ?, ?)", m.config.TableName),
		record.ID, record.Version, record.AppliedAt, record.Duration.Milliseconds(), record.Status, record.Error,
	)
}

func (m *Migrator) getAppliedMigrations(ctx context.Context) ([]*MigrationRecord, error) {
	query := fmt.Sprintf("SELECT id, version, applied_at, duration_ms, status, error FROM %s ORDER BY version", m.config.TableName)

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query applied migrations: %w", err)
	}
	defer rows.Close()

	var records []*MigrationRecord
	for rows.Next() {
		var record MigrationRecord
		var durationMs sql.NullInt64
		var errMsg sql.NullString

		if err := rows.Scan(&record.ID, &record.Version, &record.AppliedAt, &durationMs, &record.Status, &errMsg); err != nil {
			return nil, fmt.Errorf("scan migration record: %w", err)
		}

		if durationMs.Valid {
			record.Duration = time.Duration(durationMs.Int64) * time.Millisecond
		}
		if errMsg.Valid {
			record.Error = errMsg.String
		}

		records = append(records, &record)
	}

	return records, nil
}

func (m *Migrator) getPendingMigrations(applied []*MigrationRecord) []*Migration {
	appliedSet := make(map[string]bool)
	for _, record := range applied {
		appliedSet[record.ID] = true
	}

	var pending []*Migration
	for _, migration := range m.migrations {
		if !appliedSet[migration.ID] {
			pending = append(pending, migration)
		}
	}

	return pending
}

func (m *Migrator) acquireLock(ctx context.Context) error {
	lockSQL := fmt.Sprintf(`
		INSERT INTO %s (id, locked_at, locked_by)
		VALUES (1, ?, ?)
		ON DUPLICATE KEY UPDATE
			locked_at = VALUES(locked_at),
			locked_by = VALUES(locked_by)
	`, m.config.LockTableName)

	for i := 0; i < m.config.RetryCount; i++ {
		result, err := m.db.ExecContext(ctx, lockSQL, time.Now(), "migrator")
		if err != nil {
			return fmt.Errorf("acquire lock: %w", err)
		}

		if rows, _ := result.RowsAffected(); rows > 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.config.RetryDelay):
		}
	}

	return ErrLockAcquireFailed
}

func (m *Migrator) releaseLock(ctx context.Context) error {
	releaseSQL := fmt.Sprintf("DELETE FROM %s WHERE id = 1", m.config.LockTableName)
	_, err := m.db.ExecContext(ctx, releaseSQL)
	return err
}

func (m *Migrator) Validate(ctx context.Context) error {
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	appliedSet := make(map[string]bool)
	for _, record := range applied {
		appliedSet[record.ID] = true
	}

	for _, migration := range m.migrations {
		if !appliedSet[migration.ID] {
			return fmt.Errorf("%w: %s", ErrMigrationNotApplied, migration.ID)
		}
	}

	return nil
}

func (m *Migrator) CreateMigration(name, version string) *Migration {
	return &Migration{
		ID:        fmt.Sprintf("%s_%s", time.Now().Format("20060102150405"), name),
		Name:      name,
		Version:   version,
		CreatedAt: time.Now(),
	}
}
