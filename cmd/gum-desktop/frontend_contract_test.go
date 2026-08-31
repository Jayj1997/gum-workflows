//go:build darwin

package main

import (
	"os/exec"
	"testing"
)

func TestFrontendWorkflowClientContract(t *testing.T) {
	t.Parallel()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required to test the desktop frontend")
	}
	cmd := exec.Command(node, "--test", "frontend/test/workflow_client.test.mjs")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("frontend contract: %v\n%s", err, output)
	}
}
