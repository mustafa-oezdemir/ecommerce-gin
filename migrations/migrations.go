package migrations

import (
	_ "embed"
	"strings"

	"gorm.io/gorm"
)

//go:embed 000001_initial.sql
var initialSchema string

func Apply(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(64) PRIMARY KEY, applied_at DATETIME(3) NOT NULL)").Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", "000001_initial").Scan(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		for _, statement := range strings.Split(initialSchema, ";\n") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return tx.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(3))", "000001_initial").Error
	})
}
