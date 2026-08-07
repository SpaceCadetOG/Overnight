package live

import "testing"

func TestReconciliationIgnoresDryRuns(t *testing.T) {
	result := Reconcile([]Intent{{Symbol: "BTC", State: DryRun}}, nil)
	if !result.Matched {
		t.Fatalf("result=%+v", result)
	}
}

func TestReconciliationFindsUnexpectedPosition(t *testing.T) {
	result := Reconcile(nil, []string{"KAITO"})
	if result.Matched || len(result.Unexpected) != 1 {
		t.Fatalf("result=%+v", result)
	}
}
