package db

import "testing"

func TestSQLRequiresDatabaseHandle(t *testing.T) {
	if _, err := SQL(nil); err == nil {
		t.Fatal("expected nil database handle to fail")
	}
}
