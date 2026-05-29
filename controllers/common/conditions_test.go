package common_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	skillv1alpha1 "github.com/ai-keeper/ai-keeper/api/skill/v1alpha1"
	"github.com/ai-keeper/ai-keeper/controllers/common"
)

func TestSetCondition_OnTypedObject(t *testing.T) {
	t.Parallel()

	skill := &skillv1alpha1.Skill{}

	// Initial set must mutate.
	if !common.SetCondition(skill, skillv1alpha1.SkillSchemaValid,
		string(metav1.ConditionTrue), "SchemaOK", "input/output schemas valid") {
		t.Fatal("expected first SetCondition call to mutate, got false")
	}
	if got := len(skill.Status.Conditions); got != 1 {
		t.Fatalf("len(conditions) = %d, want 1", got)
	}
	c := common.GetCondition(skill, skillv1alpha1.SkillSchemaValid)
	if c == nil {
		t.Fatal("GetCondition returned nil for SchemaValid")
	}
	if c.Status != metav1.ConditionTrue || c.Reason != "SchemaOK" {
		t.Fatalf("unexpected condition state: %+v", *c)
	}
	first := c.LastTransitionTime

	// Same input → no mutation, transition time stable.
	if common.SetCondition(skill, skillv1alpha1.SkillSchemaValid,
		string(metav1.ConditionTrue), "SchemaOK", "input/output schemas valid") {
		t.Fatal("expected idempotent SetCondition to return false on identical input")
	}
	c = common.GetCondition(skill, skillv1alpha1.SkillSchemaValid)
	if !c.LastTransitionTime.Equal(&first) {
		t.Fatalf("LastTransitionTime should be stable on identical input, before=%v after=%v", first, c.LastTransitionTime)
	}

	// Status flip → mutates and bumps the timer.
	if !common.SetCondition(skill, skillv1alpha1.SkillSchemaValid,
		string(metav1.ConditionFalse), "InvalidSchema", "missing required field") {
		t.Fatal("status flip should mutate")
	}
	c = common.GetCondition(skill, skillv1alpha1.SkillSchemaValid)
	if c.Status != metav1.ConditionFalse {
		t.Fatalf("status not updated: %+v", *c)
	}
}

func TestIsReady_OnTypedObject(t *testing.T) {
	t.Parallel()

	skill := &skillv1alpha1.Skill{}
	if common.IsReady(skill) {
		t.Fatal("brand-new Skill should not be Ready")
	}

	common.SetCondition(skill, skillv1alpha1.SkillReady, string(metav1.ConditionFalse), "Pending", "")
	if common.IsReady(skill) {
		t.Fatal("Ready=False should not satisfy IsReady")
	}

	common.SetCondition(skill, skillv1alpha1.SkillReady, string(metav1.ConditionTrue), "AllConditionsTrue", "")
	if !common.IsReady(skill) {
		t.Fatal("Ready=True should satisfy IsReady")
	}

	common.SetCondition(skill, skillv1alpha1.SkillReady, string(metav1.ConditionUnknown), "Unknown", "")
	if common.IsReady(skill) {
		t.Fatal("Ready=Unknown should not satisfy IsReady")
	}
}

func TestRemoveCondition_OnTypedObject(t *testing.T) {
	t.Parallel()

	skill := &skillv1alpha1.Skill{}
	common.SetCondition(skill, skillv1alpha1.SkillRegistered, string(metav1.ConditionTrue), "OK", "")

	if !common.RemoveCondition(skill, skillv1alpha1.SkillRegistered) {
		t.Fatal("RemoveCondition should report true when condition exists")
	}
	if got := len(skill.Status.Conditions); got != 0 {
		t.Fatalf("conditions slice should be empty after remove, got len=%d", got)
	}
	if common.RemoveCondition(skill, skillv1alpha1.SkillRegistered) {
		t.Fatal("RemoveCondition on absent type should return false")
	}
}

func TestConditions_NilReceiver(t *testing.T) {
	t.Parallel()

	// IsReady / GetCondition / RemoveCondition tolerate a typed-nil
	// receiver because GetConditions returns nil and the subsequent
	// shared helpers degrade gracefully. SetCondition is intentionally
	// not in this contract — callers should never push state onto a nil
	// object.
	var skill *skillv1alpha1.Skill // typed nil
	if common.IsReady(skill) {
		t.Fatal("IsReady on nil receiver must be false")
	}
	if common.GetCondition(skill, "x") != nil {
		t.Fatal("GetCondition on nil receiver must return nil")
	}
	if common.RemoveCondition(skill, "x") {
		t.Fatal("RemoveCondition on nil receiver must return false")
	}

	// Untyped nil (i.e. a nil ConditionsAware interface) must short-
	// circuit every helper without panicking.
	if common.SetCondition(nil, "x", "True", "", "") {
		t.Fatal("SetCondition on untyped nil must return false")
	}
	if common.IsReady(nil) {
		t.Fatal("IsReady on untyped nil must return false")
	}
}
