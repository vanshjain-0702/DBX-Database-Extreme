package query

import (
	"strings"
	"testing"
)

func TestVSearchMinScoreAndSpace(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	if got := executeForTest(t, executor, "VADD", "mem", "a", "1", "0"); got != ":1\r\n" {
		t.Fatalf("vadd a = %q", got)
	}
	if got := executeForTest(t, executor, "VADD", "mem", "b", "0", "1"); got != ":1\r\n" {
		t.Fatalf("vadd b = %q", got)
	}
	all := executeForTest(t, executor, "VSEARCH", "mem", "1", "0", "5")
	if !strings.Contains(all, "a") || !strings.Contains(all, "b") {
		t.Fatalf("unfiltered search = %q", all)
	}
	filtered := executeForTest(t, executor, "VSEARCH", "mem", "1", "0", "5", "MIN_SCORE", "0.9")
	if !strings.Contains(filtered, "a") {
		t.Fatalf("min_score missing a: %q", filtered)
	}
	if strings.Contains(filtered, "b") {
		t.Fatalf("min_score leaked orthogonal hit: %q", filtered)
	}

	if got := executeForTest(t, executor, "VADD", "mem", "clip1", "SPACE", "image", "0", "1"); got != ":1\r\n" {
		t.Fatalf("vadd space = %q", got)
	}
	spaceHit := executeForTest(t, executor, "VSEARCH", "mem", "0", "1", "3", "SPACE", "image")
	if !strings.Contains(spaceHit, "clip1") {
		t.Fatalf("space search = %q", spaceHit)
	}
	defaultHit := executeForTest(t, executor, "VSEARCH", "mem", "0", "1", "3")
	if strings.Contains(defaultHit, "clip1") {
		t.Fatalf("default search leaked space id: %q", defaultHit)
	}
}

func TestVSIMExcludesSelf(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	_ = executeForTest(t, executor, "VADD", "mem", "a", "1", "0")
	_ = executeForTest(t, executor, "VADD", "mem", "b", "0.9", "0.1")
	_ = executeForTest(t, executor, "VADD", "mem", "c", "0", "1")
	got := executeForTest(t, executor, "VSIM", "mem", "a", "5")
	if strings.Contains(got, "a\r\n") {
		t.Fatalf("VSIM returned self: %q", got)
	}
	if !strings.Contains(got, "b") {
		t.Fatalf("VSIM missed neighbor: %q", got)
	}
	missing := executeForTest(t, executor, "VSIM", "mem", "nope", "3")
	if !strings.Contains(missing, "vector id not found") {
		t.Fatalf("missing id = %q", missing)
	}
}

func TestVFUSERanksAgreement(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	adds := [][]string{
		{"VADD", "mem", "agree", "SPACE", "text", "1", "0"},
		{"VADD", "mem", "agree", "SPACE", "image", "1", "0"},
		{"VADD", "mem", "split", "SPACE", "text", "1", "0"},
		{"VADD", "mem", "split", "SPACE", "image", "0", "1"},
		{"VADD", "mem", "other", "SPACE", "text", "0", "1"},
		{"VADD", "mem", "other", "SPACE", "image", "0", "1"},
	}
	for _, args := range adds {
		if got := executeForTest(t, executor, args[0], args[1:]...); !strings.Contains(got, ":1") {
			t.Fatalf("add %v = %q", args, got)
		}
	}
	got := executeForTest(t, executor, "VFUSE", "mem",
		"SPACE", "text", "1", "0",
		"SPACE", "image", "1", "0",
		"3",
		"WEIGHTS", "1,1",
	)
	agreeAt := strings.Index(got, "agree")
	splitAt := strings.Index(got, "split")
	if agreeAt < 0 || splitAt < 0 || agreeAt > splitAt {
		t.Fatalf("fused ranking = %q", got)
	}
}

func TestVSearchWithDocsAndFilterStillWork(t *testing.T) {
	executor, wal := newDurableTestExecutor(t)
	defer wal.Close()
	_ = executeForTest(t, executor, "VADD", "mem", "a", "1", "0")
	_ = executeForTest(t, executor, "SET", "doc:mem:a", "enterprise memory")
	got := executeForTest(t, executor, "VSEARCH", "mem", "1", "0", "3", "WITHDOCS", "doc:mem", "FILTER_CONTAINS", "enterprise")
	if !strings.Contains(got, "enterprise memory") {
		t.Fatalf("WITHDOCS/FILTER = %q", got)
	}
	miss := executeForTest(t, executor, "VSEARCH", "mem", "1", "0", "3", "FILTER_CONTAINS", "payroll")
	if strings.Contains(miss, "\na\r\n") {
		t.Fatalf("filter should drop a: %q", miss)
	}
}
