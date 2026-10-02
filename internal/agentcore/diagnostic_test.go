package agentcore

import (
	"testing"
)

func TestDiagnosticState_AddError(t *testing.T) {
	d := NewDiagnosticState()

	if d.HasErrors() {
		t.Error("expected no errors initially")
	}
	if d.ErrorCount() != 0 {
		t.Errorf("expected 0 errors, got %d", d.ErrorCount())
	}

	d.AddError("bash", "tool", "command not found")
	if !d.HasErrors() {
		t.Error("expected errors after adding one")
	}
	if d.ErrorCount() != 1 {
		t.Errorf("expected 1 error, got %d", d.ErrorCount())
	}

	// Add more errors
	d.AddError("read", "api", "file not found")
	d.AddError("edit", "timeout", "operation timed out")

	// Should keep only last 5
	for i := 0; i < 10; i++ {
		d.AddError("test", "tool", "error")
	}

	if d.ErrorCount() != 5 {
		t.Errorf("expected 5 errors (max), got %d", d.ErrorCount())
	}
}

func TestDiagnosticState_Errors(t *testing.T) {
	d := NewDiagnosticState()
	d.AddError("bash", "tool", "first error")
	d.AddError("read", "api", "second error")

	errs := d.Errors()
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}
	if errs[0].ToolName != "bash" {
		t.Errorf("expected first error tool=bash, got %s", errs[0].ToolName)
	}
	if errs[1].ToolName != "read" {
		t.Errorf("expected second error tool=read, got %s", errs[1].ToolName)
	}
}

func TestDiagnosticState_ReturnsCopy(t *testing.T) {
	d := NewDiagnosticState()
	d.AddError("bash", "tool", "error")

	errs1 := d.Errors()
	d.AddError("read", "api", "another error")
	errs2 := d.Errors()

	// errs1 should not be affected
	if len(errs1) != 1 {
		t.Error("expected errs1 to be a copy, not affected by later changes")
	}
	if len(errs2) != 2 {
		t.Errorf("expected errs2 to have 2 errors, got %d", len(errs2))
	}
}
