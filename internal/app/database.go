package app

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

type pgError struct {
	Code    string
	Message string
}

func (e *pgError) Error() string { return e.Message }

func isDBConstraint(err error) bool {
	var target *pgError
	return errors.As(err, &target) && strings.HasPrefix(target.Code, "23")
}

type AppDB struct {
	dsn string
	db  *sql.DB
}

type dbExecutor interface {
	exec(query string, args ...any) error
	query(query string, args ...any) ([][]string, error)
}

type dbTx struct {
	db *AppDB
	tx *sql.Tx
}

func OpenAppDB(dsn string) (*AppDB, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("DATABASE_URL is required, for example postgres://kirill_study:password@127.0.0.1:5432/kirill_study?sslmode=disable")
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	poolSize := 6
	if raw := strings.TrimSpace(os.Getenv("DB_MAX_CONNS")); raw != "" {
		if n, convErr := strconv.Atoi(raw); convErr == nil && n >= 1 && n <= 50 {
			poolSize = n
		}
	}
	sqlDB.SetMaxOpenConns(poolSize)
	sqlDB.SetMaxIdleConns(poolSize)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	db := &AppDB{dsn: dsn, db: sqlDB}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("postgres connect: %w", err)
	}
	return db, nil
}

func (db *AppDB) Close() error {
	if db == nil || db.db == nil {
		return nil
	}
	return db.db.Close()
}

func (db *AppDB) Exec(query string, args ...any) error {
	if db == nil || db.db == nil {
		return errors.New("postgres database is not initialized")
	}
	_, err := db.db.Exec(postgresPlaceholders(query), args...)
	return normalizePGError(err)
}

func (db *AppDB) Query(query string, args ...any) ([][]string, error) {
	if db == nil || db.db == nil {
		return nil, errors.New("postgres database is not initialized")
	}
	rows, err := db.db.Query(postgresPlaceholders(query), args...)
	if err != nil {
		return nil, normalizePGError(err)
	}
	defer rows.Close()
	return scanRows(rows)
}

func (db *AppDB) WithTx(fn func(*dbTx) error) error {
	tx, err := db.db.Begin()
	if err != nil {
		return normalizePGError(err)
	}
	wrapped := &dbTx{db: db, tx: tx}
	if err := fn(wrapped); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return normalizePGError(err)
	}
	return nil
}

func (tx *dbTx) exec(query string, args ...any) error {
	_, err := tx.tx.Exec(postgresPlaceholders(query), args...)
	return normalizePGError(err)
}

func (tx *dbTx) query(query string, args ...any) ([][]string, error) {
	rows, err := tx.tx.Query(postgresPlaceholders(query), args...)
	if err != nil {
		return nil, normalizePGError(err)
	}
	defer rows.Close()
	return scanRows(rows)
}

func scanRows(rows *sql.Rows) ([][]string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([][]string, 0)
	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, normalizePGError(err)
		}
		row := make([]string, len(columns))
		for i, value := range values {
			if value.Valid {
				row[i] = value.String
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, normalizePGError(err)
	}
	return result, nil
}

func normalizePGError(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return &pgError{Code: string(pqErr.Code), Message: pqErr.Message}
	}
	return err
}

func postgresPlaceholders(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 16)
	index := 1
	inSingle := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			if inSingle && i+1 < len(query) && query[i+1] == '\'' {
				b.WriteByte(ch)
				b.WriteByte(query[i+1])
				i++
				continue
			}
			inSingle = !inSingle
			b.WriteByte(ch)
			continue
		}
		if ch == '?' && !inSingle {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(index))
			index++
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

// exec and query let AppDB satisfy dbExecutor alongside dbTx.
// They keep store helpers usable both inside and outside explicit transactions.
func (db *AppDB) exec(query string, args ...any) error {
	return db.Exec(query, args...)
}

func (db *AppDB) query(query string, args ...any) ([][]string, error) {
	return db.Query(query, args...)
}
