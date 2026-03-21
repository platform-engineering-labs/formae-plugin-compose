//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os/exec"
	"testing"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTargetConfig() json.RawMessage {
	return json.RawMessage(`{"Type":"Docker","Host":"unix:///var/run/docker.sock"}`)
}

func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found, skipping integration test")
	}
}

// testComposeFile returns a simple compose file for testing with nginx on the given port.
func testComposeFile(port int) string {
	return fmt.Sprintf(`services:
  web:
    image: nginx:alpine
    ports:
      - "%d:80"
`, port)
}

// randomPort returns a random port in the ephemeral range 49152-65535.
func randomPort() int {
	return 49152 + rand.IntN(65535-49152)
}

// randomProjectName returns a unique project name for test isolation.
func randomProjectName() string {
	return fmt.Sprintf("formae-test-%d", rand.IntN(1_000_000))
}

// cleanupStack removes a compose project via docker compose down.
func cleanupStack(t *testing.T, projectName string) {
	t.Helper()
	cmd := exec.Command("docker", "compose", "-p", projectName, "down", "-v", "--remove-orphans")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("cleanup warning: %s: %s", err, out)
	}
}

func TestCreate(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	p := &Plugin{}

	projectName := randomProjectName()
	port := randomPort()

	t.Cleanup(func() {
		cleanupStack(t, projectName)
	})

	props, err := json.Marshal(map[string]any{
		"projectName": projectName,
		"composeFile": testComposeFile(port),
		"endpoints": map[string]string{
			"web": "web:80",
		},
	})
	require.NoError(t, err)

	result, err := p.Create(ctx, &resource.CreateRequest{
		ResourceType: "Docker::Compose::Stack",
		Label:        projectName,
		Properties:   props,
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	require.NotNil(t, result.ProgressResult)

	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus,
		"expected success but got: %s", result.ProgressResult.StatusMessage)
	assert.Equal(t, projectName, result.ProgressResult.NativeID)

	// Verify output properties contain resolved endpoints
	require.NotEmpty(t, result.ProgressResult.ResourceProperties)
	var outProps map[string]any
	err = json.Unmarshal(result.ProgressResult.ResourceProperties, &outProps)
	require.NoError(t, err)

	endpoints, ok := outProps["endpoints"].(map[string]any)
	require.True(t, ok, "expected endpoints in output properties, got: %v", outProps)
	webURL, ok := endpoints["web"].(string)
	require.True(t, ok, "expected web endpoint to be a string")
	assert.Contains(t, webURL, "http://")
	t.Logf("Resolved web endpoint: %s", webURL)
}

func TestCreateAlreadyRunning(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	p := &Plugin{}

	projectName := randomProjectName()
	port := randomPort()

	t.Cleanup(func() {
		cleanupStack(t, projectName)
	})

	props, err := json.Marshal(map[string]any{
		"projectName": projectName,
		"composeFile": testComposeFile(port),
		"endpoints": map[string]string{
			"web": "web:80",
		},
	})
	require.NoError(t, err)

	req := &resource.CreateRequest{
		ResourceType: "Docker::Compose::Stack",
		Label:        projectName,
		Properties:   props,
		TargetConfig: testTargetConfig(),
	}

	// First create
	result1, err := p.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result1.ProgressResult)
	assert.Equal(t, resource.OperationStatusSuccess, result1.ProgressResult.OperationStatus,
		"first create failed: %s", result1.ProgressResult.StatusMessage)

	// Second create (idempotent) — should still succeed
	result2, err := p.Create(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result2.ProgressResult)
	assert.Equal(t, resource.OperationStatusSuccess, result2.ProgressResult.OperationStatus,
		"second create (idempotent) failed: %s", result2.ProgressResult.StatusMessage)
	assert.Equal(t, projectName, result2.ProgressResult.NativeID)
}

func TestRead(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	p := &Plugin{}

	projectName := randomProjectName()
	port := randomPort()

	t.Cleanup(func() {
		cleanupStack(t, projectName)
	})

	// Create the stack first.
	props, err := json.Marshal(map[string]any{
		"projectName": projectName,
		"composeFile": testComposeFile(port),
		"endpoints": map[string]string{
			"web": "web:80",
		},
	})
	require.NoError(t, err)

	createResult, err := p.Create(ctx, &resource.CreateRequest{
		ResourceType: "Docker::Compose::Stack",
		Label:        projectName,
		Properties:   props,
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
		"create failed: %s", createResult.ProgressResult.StatusMessage)

	// Read the stack back.
	result, err := p.Read(ctx, &resource.ReadRequest{
		NativeID:     projectName,
		ResourceType: "Docker::Compose::Stack",
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	assert.Empty(t, result.ErrorCode, "expected no error code, got: %s", result.ErrorCode)
	require.NotEmpty(t, result.Properties, "expected non-empty properties")

	// Parse and verify the returned properties.
	var readProps map[string]any
	err = json.Unmarshal([]byte(result.Properties), &readProps)
	require.NoError(t, err)

	assert.Equal(t, projectName, readProps["projectName"])
	assert.NotEmpty(t, readProps["composeFile"], "expected composeFile to be present")
	assert.Equal(t, "running", readProps["status"])

	endpoints, ok := readProps["endpoints"].(map[string]any)
	require.True(t, ok, "expected endpoints in read properties, got: %v", readProps)
	webURL, ok := endpoints["web"].(string)
	require.True(t, ok, "expected web endpoint to be a string")
	assert.Contains(t, webURL, "http://")
	t.Logf("Read resolved web endpoint: %s", webURL)
}

func TestReadNotFound(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	p := &Plugin{}

	result, err := p.Read(ctx, &resource.ReadRequest{
		NativeID:     "formae-nonexistent-project-xyz",
		ResourceType: "Docker::Compose::Stack",
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode)
}

// testComposeFileWithLabel returns a compose file with an extra label on the service.
func testComposeFileWithLabel(port int, label string) string {
	return fmt.Sprintf(`services:
  web:
    image: nginx:alpine
    ports:
      - "%d:80"
    labels:
      formae.test: "%s"
`, port, label)
}

func TestUpdate(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	p := &Plugin{}

	projectName := randomProjectName()
	port := randomPort()

	t.Cleanup(func() {
		cleanupStack(t, projectName)
	})

	// Create the stack first.
	originalCompose := testComposeFile(port)
	createProps, err := json.Marshal(map[string]any{
		"projectName": projectName,
		"composeFile": originalCompose,
		"endpoints": map[string]string{
			"web": "web:80",
		},
	})
	require.NoError(t, err)

	createResult, err := p.Create(ctx, &resource.CreateRequest{
		ResourceType: "Docker::Compose::Stack",
		Label:        projectName,
		Properties:   createProps,
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
		"create failed: %s", createResult.ProgressResult.StatusMessage)

	// Prepare prior properties (from Create result) and desired properties with a changed compose file.
	priorProps := createResult.ProgressResult.ResourceProperties

	updatedCompose := testComposeFileWithLabel(port, "updated")
	desiredProps, err := json.Marshal(map[string]any{
		"projectName": projectName,
		"composeFile": updatedCompose,
		"endpoints": map[string]string{
			"web": "web:80",
		},
	})
	require.NoError(t, err)

	// Call Update.
	result, err := p.Update(ctx, &resource.UpdateRequest{
		NativeID:          projectName,
		ResourceType:      "Docker::Compose::Stack",
		PriorProperties:   priorProps,
		DesiredProperties: desiredProps,
		TargetConfig:      testTargetConfig(),
	})
	require.NoError(t, err)
	require.NotNil(t, result.ProgressResult)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus,
		"expected success but got: %s", result.ProgressResult.StatusMessage)

	// Verify returned properties reflect the new state.
	require.NotEmpty(t, result.ProgressResult.ResourceProperties)
	var outProps map[string]any
	err = json.Unmarshal(result.ProgressResult.ResourceProperties, &outProps)
	require.NoError(t, err)

	assert.Equal(t, projectName, outProps["projectName"])
	assert.Equal(t, updatedCompose, outProps["composeFile"])
	assert.Equal(t, "running", outProps["status"])

	endpoints, ok := outProps["endpoints"].(map[string]any)
	require.True(t, ok, "expected endpoints in output properties")
	webURL, ok := endpoints["web"].(string)
	require.True(t, ok, "expected web endpoint to be a string")
	assert.Contains(t, webURL, "http://")
}

func TestDelete(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	p := &Plugin{}

	projectName := randomProjectName()
	port := randomPort()

	// Create the stack first.
	props, err := json.Marshal(map[string]any{
		"projectName": projectName,
		"composeFile": testComposeFile(port),
		"endpoints": map[string]string{
			"web": "web:80",
		},
	})
	require.NoError(t, err)

	createResult, err := p.Create(ctx, &resource.CreateRequest{
		ResourceType: "Docker::Compose::Stack",
		Label:        projectName,
		Properties:   props,
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
		"create failed: %s", createResult.ProgressResult.StatusMessage)

	// Delete it.
	result, err := p.Delete(ctx, &resource.DeleteRequest{
		NativeID:     projectName,
		ResourceType: "Docker::Compose::Stack",
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	require.NotNil(t, result.ProgressResult)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus,
		"expected success but got: %s", result.ProgressResult.StatusMessage)

	// Verify: Read returns NotFound after delete.
	readResult, err := p.Read(ctx, &resource.ReadRequest{
		NativeID:     projectName,
		ResourceType: "Docker::Compose::Stack",
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	assert.Equal(t, resource.OperationErrorCodeNotFound, readResult.ErrorCode)
}

func TestDeleteNotFound(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	p := &Plugin{}

	result, err := p.Delete(ctx, &resource.DeleteRequest{
		NativeID:     "formae-nonexistent-project-for-delete",
		ResourceType: "Docker::Compose::Stack",
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	require.NotNil(t, result.ProgressResult)
	assert.Equal(t, resource.OperationStatusFailure, result.ProgressResult.OperationStatus)
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ProgressResult.ErrorCode)
}

func TestList(t *testing.T) {
	skipIfNoDocker(t)
	ctx := context.Background()
	p := &Plugin{}

	projectName := randomProjectName()
	port := randomPort()

	t.Cleanup(func() {
		cleanupStack(t, projectName)
	})

	// Create a stack to ensure at least one exists.
	props, err := json.Marshal(map[string]any{
		"projectName": projectName,
		"composeFile": testComposeFile(port),
		"endpoints": map[string]string{
			"web": "web:80",
		},
	})
	require.NoError(t, err)

	createResult, err := p.Create(ctx, &resource.CreateRequest{
		ResourceType: "Docker::Compose::Stack",
		Label:        projectName,
		Properties:   props,
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	require.Equal(t, resource.OperationStatusSuccess, createResult.ProgressResult.OperationStatus,
		"create failed: %s", createResult.ProgressResult.StatusMessage)

	// Call List.
	result, err := p.List(ctx, &resource.ListRequest{
		ResourceType: "Docker::Compose::Stack",
		TargetConfig: testTargetConfig(),
	})
	require.NoError(t, err)
	assert.Contains(t, result.NativeIDs, projectName)
}
