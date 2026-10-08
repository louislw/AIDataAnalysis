package sqlguard

import "testing"

func TestCheckDetailLimitAndSensitiveColumns(t *testing.T) {
	allowed := []string{"employee"}
	sensitive := map[string][]string{"employee": {"salary"}}

	got, err := Check("SELECT name, dept_id FROM employee WHERE dept_id = 'D3'", allowed, sensitive, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Detail || got.SQL != "SELECT name, dept_id FROM employee WHERE dept_id = 'D3' LIMIT 50" {
		t.Fatalf("detail sql = %q detail=%v", got.SQL, got.Detail)
	}

	got, err = Check("SELECT dept_id, COUNT(*) FROM employee GROUP BY dept_id", allowed, sensitive, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got.Detail || got.SQL != "SELECT dept_id, COUNT(*) FROM employee GROUP BY dept_id" {
		t.Fatalf("aggregate sql = %q detail=%v", got.SQL, got.Detail)
	}

	if _, err := Check("SELECT salary FROM employee", allowed, sensitive, 50); err == nil {
		t.Fatal("expected sensitive select to fail")
	}
	if _, err := Check("SELECT name FROM employee WHERE salary > 1", allowed, sensitive, 50); err == nil {
		t.Fatal("expected sensitive filter to fail")
	}
	if _, err := Check("SELECT name FROM employee GROUP BY salary", allowed, sensitive, 50); err == nil {
		t.Fatal("expected sensitive group by to fail")
	}
	if _, err := Check("SELECT name FROM employee ORDER BY salary", allowed, sensitive, 50); err == nil {
		t.Fatal("expected sensitive order by to fail")
	}
	got, err = Check("SELECT SUM(salary) AS total FROM employee", allowed, sensitive, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got.Detail {
		t.Fatalf("sum should be aggregate: %s", got.SQL)
	}
	if _, err := Check("SELECT * FROM employee", allowed, sensitive, 50); err == nil {
		t.Fatal("expected star to fail")
	}
	if _, err := Check("SELECT name FROM src.employee", allowed, sensitive, 50); err == nil {
		t.Fatal("expected src access to fail")
	}
	if _, err := Check("SELECT name FROM payroll", allowed, sensitive, 50); err == nil {
		t.Fatal("expected unauthorized table to fail")
	}
	if _, err := Check("SELECT name FROM employee; DROP TABLE employee", allowed, sensitive, 50); err == nil {
		t.Fatal("expected multi statement to fail")
	}
	if _, err := Check("INSERT INTO employee (name) VALUES ('x')", allowed, sensitive, 50); err == nil {
		t.Fatal("expected insert to fail")
	}

	got, err = Check("SELECT name FROM employee LIMIT 1000", allowed, sensitive, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got.SQL != "SELECT name FROM employee LIMIT 50" {
		t.Fatalf("capped sql = %q", got.SQL)
	}
}

func TestRowFilter(t *testing.T) {
	if err := ValidateRowFilter("dept_id = {{user.dept}} AND status = {{user.id}}"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRowFilter("dept_id = 'D3'"); err == nil {
		t.Fatal("expected literal to be rejected")
	}
	sql, err := CompileRowFilter("dept_id = {{user.dept}}", map[string]string{"dept": "D'3"})
	if err != nil {
		t.Fatal(err)
	}
	if sql != `"dept_id" = 'D''3'` {
		t.Fatalf("compiled = %s", sql)
	}
}
