// Package handler defines the resource handler interface and registry.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"

	"github.com/platform-engineering-labs/formae-plugin-compose/pkg/config"
)

// ResourceHandler defines CRUD+List operations for a single resource type.
type ResourceHandler interface {
	Create(ctx context.Context, cfg *config.TargetConfig, props json.RawMessage) (*resource.ProgressResult, error)
	Read(ctx context.Context, cfg *config.TargetConfig, nativeID string) (*resource.ReadResult, error)
	Update(ctx context.Context, cfg *config.TargetConfig, nativeID string, prior, desired json.RawMessage) (*resource.ProgressResult, error)
	Delete(ctx context.Context, cfg *config.TargetConfig, nativeID string) (*resource.ProgressResult, error)
	List(ctx context.Context, cfg *config.TargetConfig, pageSize int32, pageToken *string) (*resource.ListResult, error)
}

// Registry maps resource type strings to their handlers.
var Registry = map[string]ResourceHandler{}

// Register adds a handler for a resource type.
func Register(resourceType string, h ResourceHandler) {
	Registry[resourceType] = h
}

// Get returns the handler for the given resource type.
func Get(resourceType string) (ResourceHandler, error) {
	h, ok := Registry[resourceType]
	if !ok {
		return nil, fmt.Errorf("no handler registered for resource type %q", resourceType)
	}
	return h, nil
}

// MapExecError maps docker compose CLI exit codes to formae error codes.
func MapExecError(err error) resource.OperationErrorCode {
	if err == nil {
		return ""
	}

	errStr := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errStr, "not found"):
		return resource.OperationErrorCodeNotFound
	case strings.Contains(errStr, "permission denied"):
		return resource.OperationErrorCodeAccessDenied
	case strings.Contains(errStr, "already exists"):
		return resource.OperationErrorCodeAlreadyExists
	case strings.Contains(errStr, "conflict"):
		return resource.OperationErrorCodeResourceConflict
	default:
		return resource.OperationErrorCodeInternalFailure
	}
}

// FailResult creates a failure ProgressResult for the given operation.
func FailResult(op resource.Operation, code resource.OperationErrorCode, msg string) *resource.ProgressResult {
	return &resource.ProgressResult{
		Operation:       op,
		OperationStatus: resource.OperationStatusFailure,
		ErrorCode:       code,
		StatusMessage:   msg,
	}
}

// SuccessResult creates a success ProgressResult for the given operation.
func SuccessResult(op resource.Operation, nativeID string, props json.RawMessage) *resource.ProgressResult {
	return &resource.ProgressResult{
		Operation:          op,
		OperationStatus:    resource.OperationStatusSuccess,
		NativeID:           nativeID,
		ResourceProperties: props,
	}
}
