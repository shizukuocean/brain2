package brain2

import "testing"

func TestHealthSummarySeparatesDeterministicLayersAndCompactsMCPPayload(t *testing.T) {
	report := HealthReport{Checks: []CheckResult{
		{Name: ".brain2/config.json", Path: ".brain2/config.json", Status: "ok"},
		{Name: "project_record:demo", Path: ".brain2/project-records/demo.json", Status: "warning"},
		{Name: "孤立候选：note.md", Path: "note.md", Status: "info", Category: "orphan_candidate"},
		{Name: "断链：missing", Path: "note.md", Status: "error", Category: "broken_link"},
	}}
	finalizeHealthSummary(&report)
	if report.FindingsTotal != 4 || len(report.Categories) < 3 {
		t.Fatalf("unexpected health summary: %#v", report)
	}
	if report.Checks[0].Category != "connection_core" || report.Checks[1].Category != "project_registry" {
		t.Fatalf("deterministic layers were not classified: %#v", report.Checks)
	}
	compact := compactHealthReport(report, 2)
	if !compact.ChecksTruncated || compact.ReturnedChecks != 2 || compact.FindingsTotal != 4 {
		t.Fatalf("unexpected compact report: %#v", compact)
	}
	for _, check := range compact.Checks {
		if check.Status == "ok" {
			t.Fatalf("compact payload should prioritize findings: %#v", compact.Checks)
		}
	}
}
