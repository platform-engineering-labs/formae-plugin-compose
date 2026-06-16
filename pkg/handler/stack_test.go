package handler

import (
	"testing"
)

func TestComposeEnv_MergesVariables(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HOME=/root"}
	vars := map[string]string{"VLLM_HOST": "1.2.3.4", "MODEL": "chat"}
	got := composeEnv(base, vars)

	// base entries preserved
	if !contains(got, "PATH=/usr/bin") || !contains(got, "HOME=/root") {
		t.Fatalf("base env not preserved: %v", got)
	}
	// variables appended in KEY=VALUE form
	if !contains(got, "VLLM_HOST=1.2.3.4") || !contains(got, "MODEL=chat") {
		t.Fatalf("variables not merged: %v", got)
	}
}

func TestComposeEnv_NilVariables(t *testing.T) {
	base := []string{"PATH=/usr/bin"}
	got := composeEnv(base, nil)
	if len(got) != 1 || got[0] != "PATH=/usr/bin" {
		t.Fatalf("nil variables should return base unchanged: %v", got)
	}
}

func TestComposeEnv_DoesNotMutateBase(t *testing.T) {
	base := []string{"A=1"}
	_ = composeEnv(base, map[string]string{"B": "2"})
	if len(base) != 1 {
		t.Fatalf("composeEnv mutated base: %v", base)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
