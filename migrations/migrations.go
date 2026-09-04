package migrations

import (
	_ "embed"
	"strings"

	"gorm.io/gorm"
)

//go:embed 000001_initial.sql
var initialSchema string

//go:embed 000002_product_lists.sql
var productListsSchema string

//go:embed 000003_product_images.sql
var productImagesSchema string

//go:embed 000004_account_security.sql
var accountSecuritySchema string

//go:embed 000005_favorites_reviews.sql
var favoritesReviewsSchema string

//go:embed 000006_account_email_length.sql
var accountEmailLengthSchema string

//go:embed 000007_product_image_gallery.sql
var productImageGallerySchema string

type migration struct {
	version string
	sql     string
}

func Apply(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(64) PRIMARY KEY, applied_at DATETIME(3) NOT NULL)").Error; err != nil {
			return err
		}
		for _, migration := range []migration{
			{version: "000001_initial", sql: initialSchema},
			{version: "000002_product_lists", sql: productListsSchema},
			{version: "000003_product_images", sql: productImagesSchema},
			{version: "000004_account_security", sql: accountSecuritySchema},
			{version: "000005_favorites_reviews", sql: favoritesReviewsSchema},
			{version: "000006_account_email_length", sql: accountEmailLengthSchema},
			{version: "000007_product_image_gallery", sql: productImageGallerySchema},
		} {
			var count int64
			if err := tx.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", migration.version).Scan(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			for _, statement := range strings.Split(migration.sql, ";\n") {
				statement = strings.TrimSpace(statement)
				if statement == "" {
					continue
				}
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			if err := tx.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, CURRENT_TIMESTAMP(3))", migration.version).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
