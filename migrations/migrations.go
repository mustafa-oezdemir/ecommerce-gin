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

//go:embed 000008_profile_name_backfill.sql
var profileNameBackfillSchema string

//go:embed 000009_profile_images.sql
var profileImagesSchema string

//go:embed 000010_checkout_payments.sql
var checkoutPaymentsSchema string

//go:embed 000011_shipping_integration.sql
var shippingIntegrationSchema string

//go:embed 000012_shipping_order_type.sql
var shippingOrderTypeSchema string

//go:embed 000013_notifications.sql
var notificationsSchema string

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
			{version: "000008_profile_name_backfill", sql: profileNameBackfillSchema},
			{version: "000009_profile_images", sql: profileImagesSchema},
			{version: "000010_checkout_payments", sql: checkoutPaymentsSchema},
			{version: "000011_shipping_integration", sql: shippingIntegrationSchema},
			{version: "000012_shipping_order_type", sql: shippingOrderTypeSchema},
			{version: "000013_notifications", sql: notificationsSchema},
		} {
			var count int64
			if err := tx.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", migration.version).Scan(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			for _, statement := range splitStatements(migration.sql) {
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

func splitStatements(script string) []string {
	normalized := strings.ReplaceAll(script, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, ";\n")
}
