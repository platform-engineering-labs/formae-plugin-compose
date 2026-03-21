package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"

	"github.com/platform-engineering-labs/formae-plugin-compose/pkg/config"
)

func init() {
	Register("Docker::Compose::Stack", &StackHandler{})
}

// StackHandler implements CRUD+List for Docker Compose stacks.
type StackHandler struct{}

// stackProps represents the properties of a Docker Compose stack resource.
type stackProps struct {
	ProjectName string            `json:"projectName"`
	ComposeFile string            `json:"composeFile"`
	Endpoints   map[string]string `json:"endpoints"`
	Status      string            `json:"status,omitempty"`
}

func (h *StackHandler) Create(ctx context.Context, _ *config.TargetConfig, rawProps json.RawMessage) (*resource.ProgressResult, error) {
	var props stackProps
	if err := json.Unmarshal(rawProps, &props); err != nil {
		return FailResult(resource.OperationCreate, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid properties: %v", err)), nil
	}

	if props.ProjectName == "" {
		return FailResult(resource.OperationCreate, resource.OperationErrorCodeInvalidRequest,
			"projectName is required"), nil
	}
	if props.ComposeFile == "" {
		return FailResult(resource.OperationCreate, resource.OperationErrorCodeInvalidRequest,
			"composeFile is required"), nil
	}

	// Write compose file to a predictable temp location.
	composeDir := filepath.Join(os.TempDir(), "formae-compose-"+props.ProjectName)
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create compose dir: %w", err)
	}
	composePath := filepath.Join(composeDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(props.ComposeFile), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write compose file: %w", err)
	}

	// Persist endpoint declarations so Read can re-resolve them.
	if len(props.Endpoints) > 0 {
		endpointsJSON, err := json.Marshal(props.Endpoints)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal endpoint declarations: %w", err)
		}
		endpointsPath := filepath.Join(composeDir, "endpoints.json")
		if err := os.WriteFile(endpointsPath, endpointsJSON, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write endpoints file: %w", err)
		}
	}

	// Run docker compose up.
	if _, err := runCompose(ctx, props.ProjectName, "-f", composePath, "up", "-d", "--wait"); err != nil {
		return FailResult(resource.OperationCreate, MapExecError(err),
			fmt.Sprintf("docker compose up failed: %v", err)), nil
	}

	// Resolve endpoints.
	resolvedEndpoints := make(map[string]string, len(props.Endpoints))
	for name, declaration := range props.Endpoints {
		resolved, err := resolveEndpoint(ctx, props.ProjectName, declaration)
		if err != nil {
			return FailResult(resource.OperationCreate, resource.OperationErrorCodeInternalFailure,
				fmt.Sprintf("failed to resolve endpoint %q: %v", name, err)), nil
		}
		resolvedEndpoints[name] = resolved
	}

	// Build output properties.
	outProps := stackProps{
		ProjectName: props.ProjectName,
		ComposeFile: props.ComposeFile,
		Endpoints:   resolvedEndpoints,
		Status:      "running",
	}
	outJSON, err := json.Marshal(outProps)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal output properties: %w", err)
	}

	return SuccessResult(resource.OperationCreate, props.ProjectName, outJSON), nil
}

// resolveEndpoint resolves a declared endpoint (e.g. "web:80") to an actual URL
// by querying docker compose for the published port.
func resolveEndpoint(ctx context.Context, projectName, declaration string) (string, error) {
	parts := strings.SplitN(declaration, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid endpoint declaration %q: expected service:port", declaration)
	}
	service := parts[0]
	containerPort := parts[1]

	output, err := runCompose(ctx, projectName, "port", service, containerPort)
	if err != nil {
		return "", fmt.Errorf("docker compose port failed: %w", err)
	}

	// Output is like "0.0.0.0:32768\n" or "[::]:32768\n"
	hostPort := strings.TrimSpace(output)
	if hostPort == "" {
		return "", fmt.Errorf("no published port found for %s:%s", service, containerPort)
	}

	// Replace 0.0.0.0 or [::] with localhost for usable URLs.
	hostPort = strings.Replace(hostPort, "0.0.0.0", "localhost", 1)
	hostPort = strings.Replace(hostPort, "[::]", "localhost", 1)

	return "http://" + hostPort, nil
}

func (h *StackHandler) Read(ctx context.Context, _ *config.TargetConfig, nativeID string) (*resource.ReadResult, error) {
	notFound := &resource.ReadResult{
		ResourceType: "Docker::Compose::Stack",
		ErrorCode:    resource.OperationErrorCodeNotFound,
	}

	// Check if the project exists by querying running containers.
	psOutput, err := runCompose(ctx, nativeID, "ps", "--format", "json")
	if err != nil || strings.TrimSpace(psOutput) == "" {
		return notFound, nil
	}

	// Parse container states to determine overall status and verify the project is real.
	containers, err := parseComposePS(psOutput)
	if err != nil || len(containers) == 0 {
		return notFound, nil
	}
	status := overallStatus(containers)

	// Read the stored compose file.
	composeDir := filepath.Join(os.TempDir(), "formae-compose-"+nativeID)
	composePath := filepath.Join(composeDir, "docker-compose.yml")
	composeBytes, err := os.ReadFile(composePath)
	if err != nil {
		return notFound, nil
	}

	// Read stored endpoint declarations (if any).
	endpointDeclarations := map[string]string{}
	endpointsPath := filepath.Join(composeDir, "endpoints.json")
	if data, err := os.ReadFile(endpointsPath); err == nil {
		_ = json.Unmarshal(data, &endpointDeclarations)
	}

	// Re-resolve endpoints from running state.
	resolvedEndpoints := make(map[string]string, len(endpointDeclarations))
	for name, declaration := range endpointDeclarations {
		resolved, err := resolveEndpoint(ctx, nativeID, declaration)
		if err == nil {
			resolvedEndpoints[name] = resolved
		}
	}

	outProps := stackProps{
		ProjectName: nativeID,
		ComposeFile: string(composeBytes),
		Endpoints:   resolvedEndpoints,
		Status:      status,
	}
	outJSON, err := json.Marshal(outProps)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal read properties: %w", err)
	}

	return &resource.ReadResult{
		ResourceType: "Docker::Compose::Stack",
		Properties:   string(outJSON),
	}, nil
}

// containerInfo represents a container from docker compose ps --format json output.
type containerInfo struct {
	State string `json:"State"`
}

// parseComposePS parses the JSON output of docker compose ps --format json.
// Docker compose outputs one JSON object per line (not a JSON array).
func parseComposePS(output string) ([]containerInfo, error) {
	var containers []containerInfo
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var c containerInfo
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, fmt.Errorf("failed to parse container info: %w", err)
		}
		containers = append(containers, c)
	}
	return containers, nil
}

// overallStatus determines the aggregate status from individual container states.
func overallStatus(containers []containerInfo) string {
	for _, c := range containers {
		if c.State != "running" {
			return "degraded"
		}
	}
	return "running"
}

func (h *StackHandler) Update(ctx context.Context, _ *config.TargetConfig, nativeID string, _, desiredProps json.RawMessage) (*resource.ProgressResult, error) {
	var props stackProps
	if err := json.Unmarshal(desiredProps, &props); err != nil {
		return FailResult(resource.OperationUpdate, resource.OperationErrorCodeInvalidRequest,
			fmt.Sprintf("invalid desired properties: %v", err)), nil
	}

	if props.ComposeFile == "" {
		return FailResult(resource.OperationUpdate, resource.OperationErrorCodeInvalidRequest,
			"composeFile is required"), nil
	}

	// Write updated compose file to the same temp path.
	composeDir := filepath.Join(os.TempDir(), "formae-compose-"+nativeID)
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create compose dir: %w", err)
	}
	composePath := filepath.Join(composeDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(props.ComposeFile), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write compose file: %w", err)
	}

	// Update endpoints.json if endpoints changed.
	endpointsPath := filepath.Join(composeDir, "endpoints.json")
	if len(props.Endpoints) > 0 {
		endpointsJSON, err := json.Marshal(props.Endpoints)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal endpoint declarations: %w", err)
		}
		if err := os.WriteFile(endpointsPath, endpointsJSON, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write endpoints file: %w", err)
		}
	} else {
		// Remove endpoints file if no endpoints are declared.
		_ = os.Remove(endpointsPath)
	}

	// Run docker compose up (idempotent, recreates changed services).
	if _, err := runCompose(ctx, nativeID, "-f", composePath, "up", "-d", "--wait"); err != nil {
		return FailResult(resource.OperationUpdate, MapExecError(err),
			fmt.Sprintf("docker compose up failed: %v", err)), nil
	}

	// Re-resolve endpoints.
	resolvedEndpoints := make(map[string]string, len(props.Endpoints))
	for name, declaration := range props.Endpoints {
		resolved, err := resolveEndpoint(ctx, nativeID, declaration)
		if err != nil {
			return FailResult(resource.OperationUpdate, resource.OperationErrorCodeInternalFailure,
				fmt.Sprintf("failed to resolve endpoint %q: %v", name, err)), nil
		}
		resolvedEndpoints[name] = resolved
	}

	// Build output properties.
	outProps := stackProps{
		ProjectName: props.ProjectName,
		ComposeFile: props.ComposeFile,
		Endpoints:   resolvedEndpoints,
		Status:      "running",
	}
	outJSON, err := json.Marshal(outProps)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal output properties: %w", err)
	}

	return SuccessResult(resource.OperationUpdate, nativeID, outJSON), nil
}

func (h *StackHandler) Delete(ctx context.Context, _ *config.TargetConfig, nativeID string) (*resource.ProgressResult, error) {
	// Check existence: query running containers for this project.
	psOutput, err := runCompose(ctx, nativeID, "ps", "--format", "json")
	if err != nil || strings.TrimSpace(psOutput) == "" {
		return FailResult(resource.OperationDelete, resource.OperationErrorCodeNotFound,
			fmt.Sprintf("project %q not found", nativeID)), nil
	}

	containers, err := parseComposePS(psOutput)
	if err != nil || len(containers) == 0 {
		return FailResult(resource.OperationDelete, resource.OperationErrorCodeNotFound,
			fmt.Sprintf("project %q not found", nativeID)), nil
	}

	// Run docker compose down.
	if _, err := runCompose(ctx, nativeID, "down", "-v", "--remove-orphans"); err != nil {
		return FailResult(resource.OperationDelete, MapExecError(err),
			fmt.Sprintf("docker compose down failed: %v", err)), nil
	}

	// Clean up temp files.
	composeDir := filepath.Join(os.TempDir(), "formae-compose-"+nativeID)
	_ = os.RemoveAll(composeDir)

	return SuccessResult(resource.OperationDelete, nativeID, nil), nil
}

// composeProject represents a project from docker compose ls --format json output.
type composeProject struct {
	Name string `json:"Name"`
}

func (h *StackHandler) List(ctx context.Context, _ *config.TargetConfig, _ int32, _ *string) (*resource.ListResult, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "ls", "--format", "json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	var projects []composeProject
	if err := json.Unmarshal(stdout.Bytes(), &projects); err != nil {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	nativeIDs := make([]string, 0, len(projects))
	for _, p := range projects {
		nativeIDs = append(nativeIDs, p.Name)
	}

	return &resource.ListResult{NativeIDs: nativeIDs}, nil
}

// runCompose executes a docker compose command for the given project and returns stdout.
func runCompose(ctx context.Context, projectName string, args ...string) (string, error) {
	cmdArgs := append([]string{"compose", "-p", projectName}, args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker compose failed: %s: %w", stderr.String(), err)
	}
	return stdout.String(), nil
}
