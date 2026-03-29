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

	// Write compose file to a temp location for the docker compose CLI.
	composePath, err := writeComposeFile(props.ProjectName, props.ComposeFile)
	if err != nil {
		return nil, err
	}

	// Run docker compose up.
	if _, err := runCompose(ctx, props.ProjectName, "-f", composePath, "up", "-d", "--wait"); err != nil {
		return FailResult(resource.OperationCreate, MapExecError(err),
			fmt.Sprintf("docker compose up failed: %v", err)), nil
	}

	// Discover endpoints from running containers.
	psOutput, err := runCompose(ctx, props.ProjectName, "ps", "--format", "json")
	if err != nil {
		psOutput = ""
	}
	containers, _ := parseComposePS(psOutput)
	endpoints := discoverEndpoints(containers)

	outProps := stackProps{
		ProjectName: props.ProjectName,
		ComposeFile: props.ComposeFile,
		Endpoints:   endpoints,
		Status:      "running",
	}
	outJSON, err := json.Marshal(outProps)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal output properties: %w", err)
	}

	return SuccessResult(resource.OperationCreate, props.ProjectName, outJSON), nil
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

	containers, err := parseComposePS(psOutput)
	if err != nil || len(containers) == 0 {
		return notFound, nil
	}
	status := overallStatus(containers)

	// Try to read compose file from temp cache (exists for formae-managed stacks).
	// Not an error if missing — discovered stacks won't have this.
	var composeFile string
	composeDir := filepath.Join(os.TempDir(), "formae-compose-"+nativeID)
	composePath := filepath.Join(composeDir, "docker-compose.yml")
	if data, err := os.ReadFile(composePath); err == nil {
		composeFile = string(data)
	}

	// Discover endpoints from running containers' published ports.
	endpoints := discoverEndpoints(containers)

	outProps := stackProps{
		ProjectName: nativeID,
		ComposeFile: composeFile,
		Endpoints:   endpoints,
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
	Service    string          `json:"Service"`
	State      string          `json:"State"`
	Publishers []publisherInfo `json:"Publishers"`
}

type publisherInfo struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

// discoverEndpoints builds an endpoints map from running containers' published ports.
// Keys are "service:containerPort", values are "http://localhost:publishedPort".
func discoverEndpoints(containers []containerInfo) map[string]string {
	endpoints := make(map[string]string)
	for _, c := range containers {
		for _, p := range c.Publishers {
			if p.PublishedPort == 0 || p.Protocol != "tcp" {
				continue
			}
			// Skip IPv6 duplicates (0.0.0.0 and :: both map to localhost)
			if p.URL == "::" {
				continue
			}
			key := fmt.Sprintf("%s:%d", c.Service, p.TargetPort)
			endpoints[key] = fmt.Sprintf("http://localhost:%d", p.PublishedPort)
		}
	}
	return endpoints
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

	// Write updated compose file to a temp location for the docker compose CLI.
	composePath, err := writeComposeFile(nativeID, props.ComposeFile)
	if err != nil {
		return nil, err
	}

	// Run docker compose up (idempotent, recreates changed services).
	if _, err := runCompose(ctx, nativeID, "-f", composePath, "up", "-d", "--wait"); err != nil {
		return FailResult(resource.OperationUpdate, MapExecError(err),
			fmt.Sprintf("docker compose up failed: %v", err)), nil
	}

	// Discover endpoints from running containers.
	psOutput, err := runCompose(ctx, nativeID, "ps", "--format", "json")
	if err != nil {
		psOutput = ""
	}
	updateContainers, _ := parseComposePS(psOutput)
	endpoints := discoverEndpoints(updateContainers)

	outProps := stackProps{
		ProjectName: props.ProjectName,
		ComposeFile: props.ComposeFile,
		Endpoints:   endpoints,
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

// writeComposeFile writes the compose YAML to a temp location for docker compose CLI.
// This is an ephemeral cache, not persistent state.
func writeComposeFile(projectName, content string) (string, error) {
	dir := filepath.Join(os.TempDir(), "formae-compose-"+projectName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create compose dir: %w", err)
	}
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("failed to write compose file: %w", err)
	}
	return path, nil
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
