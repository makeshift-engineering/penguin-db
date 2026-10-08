package executor

import (
	"context"
	"errors"
	"testing"
)

func TestExecuteUnsupportedPlan(t *testing.T) {
	exec, _, _ := setupExecutor(t)

	_, err := exec.Execute(context.Background(), nil)
	if !errors.Is(err, ErrUnsupportedPlan) {
		t.Fatalf("expected ErrUnsupportedPlan, got %v", err)
	}
}
