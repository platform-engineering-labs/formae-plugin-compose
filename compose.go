// © 2025 Platform Engineering Labs Inc.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"

	"github.com/platform-engineering-labs/formae/pkg/plugin"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"

	"github.com/platform-engineering-labs/formae-plugin-compose/pkg/config"
	"github.com/platform-engineering-labs/formae-plugin-compose/pkg/handler"
)

// Plugin implements the Formae ResourcePlugin interface for Docker Compose.
// The SDK automatically provides identity methods (Name, Version, Namespace)
// by reading formae-plugin.pkl at startup.
type Plugin struct{}

// Compile-time check: Plugin must satisfy ResourcePlugin interface.
var _ plugin.ResourcePlugin = &Plugin{}

// =============================================================================
// Configuration Methods
// =============================================================================

// RateLimit returns the rate limiting configuration for this plugin.
func (p *Plugin) RateLimit() plugin.RateLimitConfig {
	return plugin.RateLimitConfig{
		Scope:                            plugin.RateLimitScopeNamespace,
		MaxRequestsPerSecondForNamespace: 5,
	}
}

// DiscoveryFilters returns filters to exclude certain resources from discovery.
func (p *Plugin) DiscoveryFilters() []plugin.MatchFilter {
	return nil
}

// LabelConfig returns the configuration for extracting human-readable labels.
func (p *Plugin) LabelConfig() plugin.LabelConfig {
	return plugin.LabelConfig{
		DefaultQuery: "$.projectName",
	}
}

// parseConfig parses the target configuration from a request.
func parseConfig(targetConfig []byte) (*config.TargetConfig, error) {
	cfg, err := config.ParseTargetConfig(targetConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse target config: %w", err)
	}
	return cfg, nil
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create provisions a new resource.
func (p *Plugin) Create(ctx context.Context, req *resource.CreateRequest) (*resource.CreateResult, error) {
	h, err := handler.Get(req.ResourceType)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: handler.FailResult(resource.OperationCreate, resource.OperationErrorCodeInvalidRequest, err.Error()),
		}, nil
	}

	cfg, err := parseConfig(req.TargetConfig)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: handler.FailResult(resource.OperationCreate, resource.OperationErrorCodeInvalidRequest, err.Error()),
		}, nil
	}

	result, err := h.Create(ctx, cfg, req.Properties)
	if err != nil {
		return &resource.CreateResult{
			ProgressResult: handler.FailResult(resource.OperationCreate, resource.OperationErrorCodeInternalFailure, err.Error()),
		}, nil
	}
	return &resource.CreateResult{ProgressResult: result}, nil
}

// Read retrieves the current state of a resource.
func (p *Plugin) Read(ctx context.Context, req *resource.ReadRequest) (*resource.ReadResult, error) {
	h, err := handler.Get(req.ResourceType)
	if err != nil {
		return &resource.ReadResult{
			ResourceType: req.ResourceType,
			ErrorCode:    resource.OperationErrorCodeInvalidRequest,
		}, nil
	}

	cfg, err := parseConfig(req.TargetConfig)
	if err != nil {
		return &resource.ReadResult{
			ResourceType: req.ResourceType,
			ErrorCode:    resource.OperationErrorCodeInvalidRequest,
		}, nil
	}

	return h.Read(ctx, cfg, req.NativeID)
}

// Update modifies an existing resource.
func (p *Plugin) Update(ctx context.Context, req *resource.UpdateRequest) (*resource.UpdateResult, error) {
	h, err := handler.Get(req.ResourceType)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: handler.FailResult(resource.OperationUpdate, resource.OperationErrorCodeInvalidRequest, err.Error()),
		}, nil
	}

	cfg, err := parseConfig(req.TargetConfig)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: handler.FailResult(resource.OperationUpdate, resource.OperationErrorCodeInvalidRequest, err.Error()),
		}, nil
	}

	result, err := h.Update(ctx, cfg, req.NativeID, req.PriorProperties, req.DesiredProperties)
	if err != nil {
		return &resource.UpdateResult{
			ProgressResult: handler.FailResult(resource.OperationUpdate, resource.OperationErrorCodeInternalFailure, err.Error()),
		}, nil
	}
	return &resource.UpdateResult{ProgressResult: result}, nil
}

// Delete removes a resource.
func (p *Plugin) Delete(ctx context.Context, req *resource.DeleteRequest) (*resource.DeleteResult, error) {
	h, err := handler.Get(req.ResourceType)
	if err != nil {
		return &resource.DeleteResult{
			ProgressResult: handler.FailResult(resource.OperationDelete, resource.OperationErrorCodeInvalidRequest, err.Error()),
		}, nil
	}

	cfg, err := parseConfig(req.TargetConfig)
	if err != nil {
		return &resource.DeleteResult{
			ProgressResult: handler.FailResult(resource.OperationDelete, resource.OperationErrorCodeInvalidRequest, err.Error()),
		}, nil
	}

	result, err := h.Delete(ctx, cfg, req.NativeID)
	if err != nil {
		return &resource.DeleteResult{
			ProgressResult: handler.FailResult(resource.OperationDelete, resource.OperationErrorCodeInternalFailure, err.Error()),
		}, nil
	}
	return &resource.DeleteResult{ProgressResult: result}, nil
}

// Status checks the progress of an async operation.
// All Docker Compose operations are synchronous, so this always returns Success.
func (p *Plugin) Status(_ context.Context, req *resource.StatusRequest) (*resource.StatusResult, error) {
	return &resource.StatusResult{
		ProgressResult: &resource.ProgressResult{
			Operation:       resource.OperationCheckStatus,
			OperationStatus: resource.OperationStatusSuccess,
			RequestID:       req.RequestID,
		},
	}, nil
}

// List returns all resource identifiers of a given type for discovery.
func (p *Plugin) List(ctx context.Context, req *resource.ListRequest) (*resource.ListResult, error) {
	h, err := handler.Get(req.ResourceType)
	if err != nil {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	cfg, err := parseConfig(req.TargetConfig)
	if err != nil {
		return &resource.ListResult{NativeIDs: []string{}}, nil
	}

	return h.List(ctx, cfg, req.PageSize, req.PageToken)
}
