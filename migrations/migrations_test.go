package migrations

import "testing"

func TestSplitStatementsHandlesMixedLineEndings(t *testing.T) {
	statements := splitStatements("CREATE TABLE one (id INT);\r\nCREATE TABLE two (id INT);\n")
	if len(statements) != 3 || statements[0] != "CREATE TABLE one (id INT)" || statements[1] != "CREATE TABLE two (id INT)" {
		t.Fatalf("unexpected statements: %#v", statements)
	}
}
