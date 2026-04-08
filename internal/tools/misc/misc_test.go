package misc_test

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/claude-code-go/internal/tools"
	"github.com/anthropics/claude-code-go/internal/tools/misc"
)

// ── SkillTool ─────────────────────────────────────────────────────────────────

func TestSkillTool_Name(t *testing.T) {
	if misc.SkillTool.Name() != "Skill" {
		t.Errorf("expected Skill, got %q", misc.SkillTool.Name())
	}
}

func TestSkillTool_IsConcurrencySafe_False(t *testing.T) {
	if misc.SkillTool.IsConcurrencySafe(nil) {
		t.Error("SkillTool must not be concurrency-safe")
	}
}

func TestSkillTool_IsReadOnly_False(t *testing.T) {
	if misc.SkillTool.IsReadOnly(nil) {
		t.Error("SkillTool must not be read-only")
	}
}

func TestSkillTool_InputSchema(t *testing.T) {
	schema := misc.SkillTool.InputSchema()
	if _, ok := schema.Properties["skill"]; !ok {
		t.Error("schema missing 'skill'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "skill" {
		t.Errorf("expected Required=[skill], got %v", schema.Required)
	}
}

func TestSkillTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(misc.SkillInput{Skill: "commit"})
	name := misc.SkillTool.UserFacingName(in)
	if name != "Skill(commit)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestSkillTool_UserFacingName_NoInput(t *testing.T) {
	if misc.SkillTool.UserFacingName(nil) != "Skill" {
		t.Error("expected fallback Skill")
	}
}

func TestSkillTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(misc.SkillInput{Skill: "commit"})
	result, err := misc.SkillTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── BriefTool ─────────────────────────────────────────────────────────────────

func TestBriefTool_Name(t *testing.T) {
	if misc.BriefTool.Name() != "Brief" {
		t.Errorf("expected Brief, got %q", misc.BriefTool.Name())
	}
}

func TestBriefTool_IsConcurrencySafe_True(t *testing.T) {
	if !misc.BriefTool.IsConcurrencySafe(nil) {
		t.Error("BriefTool should be concurrency-safe")
	}
}

func TestBriefTool_IsReadOnly_True(t *testing.T) {
	if !misc.BriefTool.IsReadOnly(nil) {
		t.Error("BriefTool should be read-only")
	}
}

func TestBriefTool_InputSchema(t *testing.T) {
	schema := misc.BriefTool.InputSchema()
	if _, ok := schema.Properties["content"]; !ok {
		t.Error("schema missing 'content'")
	}
}

func TestBriefTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(misc.BriefInput{Content: "some text"})
	result, err := misc.BriefTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── ToolSearchTool ────────────────────────────────────────────────────────────

func TestToolSearchTool_Name(t *testing.T) {
	if misc.ToolSearchTool.Name() != "ToolSearch" {
		t.Errorf("expected ToolSearch, got %q", misc.ToolSearchTool.Name())
	}
}

func TestToolSearchTool_IsConcurrencySafe_True(t *testing.T) {
	if !misc.ToolSearchTool.IsConcurrencySafe(nil) {
		t.Error("ToolSearchTool should be concurrency-safe")
	}
}

func TestToolSearchTool_IsReadOnly_True(t *testing.T) {
	if !misc.ToolSearchTool.IsReadOnly(nil) {
		t.Error("ToolSearchTool should be read-only")
	}
}

func TestToolSearchTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(misc.ToolSearchInput{Query: "file read"})
	name := misc.ToolSearchTool.UserFacingName(in)
	if name != "ToolSearch(file read)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestToolSearchTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(misc.ToolSearchInput{Query: "something"})
	result, err := misc.ToolSearchTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── SleepTool ─────────────────────────────────────────────────────────────────

func TestSleepTool_Name(t *testing.T) {
	if misc.SleepTool.Name() != "Sleep" {
		t.Errorf("expected Sleep, got %q", misc.SleepTool.Name())
	}
}

func TestSleepTool_InputSchema(t *testing.T) {
	schema := misc.SleepTool.InputSchema()
	if _, ok := schema.Properties["milliseconds"]; !ok {
		t.Error("schema missing 'milliseconds'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "milliseconds" {
		t.Errorf("expected Required=[milliseconds], got %v", schema.Required)
	}
}

func TestSleepTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 500})
	name := misc.SleepTool.UserFacingName(in)
	if name != "Sleep(500ms)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestSleepTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 100})
	result, err := misc.SleepTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── SyntheticOutputTool ───────────────────────────────────────────────────────

func TestSyntheticOutputTool_Name(t *testing.T) {
	if misc.SyntheticOutputTool.Name() != "SyntheticOutput" {
		t.Errorf("expected SyntheticOutput, got %q", misc.SyntheticOutputTool.Name())
	}
}

func TestSyntheticOutputTool_IsConcurrencySafe_False(t *testing.T) {
	if misc.SyntheticOutputTool.IsConcurrencySafe(nil) {
		t.Error("SyntheticOutputTool must not be concurrency-safe")
	}
}

func TestSyntheticOutputTool_IsReadOnly_True(t *testing.T) {
	if !misc.SyntheticOutputTool.IsReadOnly(nil) {
		t.Error("SyntheticOutputTool should be read-only")
	}
}

func TestSyntheticOutputTool_InputSchema(t *testing.T) {
	schema := misc.SyntheticOutputTool.InputSchema()
	if _, ok := schema.Properties["content"]; !ok {
		t.Error("schema missing 'content'")
	}
	if _, ok := schema.Properties["is_error"]; !ok {
		t.Error("schema missing 'is_error'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "content" {
		t.Errorf("expected Required=[content], got %v", schema.Required)
	}
}

func TestSyntheticOutputTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(misc.SyntheticOutputInput{Content: "test"})
	result, err := misc.SyntheticOutputTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── Interface compliance ──────────────────────────────────────────────────────

func TestAllMiscTools_ImplementToolInterface(t *testing.T) {
	var _ tools.Tool = misc.SkillTool
	var _ tools.Tool = misc.BriefTool
	var _ tools.Tool = misc.ToolSearchTool
	var _ tools.Tool = misc.SleepTool
	var _ tools.Tool = misc.SyntheticOutputTool
}
