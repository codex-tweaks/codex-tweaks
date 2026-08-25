package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	NetworkCapabilityID                = "network"
	NetworkCapabilityVersion           = "1.0.0"
	UISettingsSectionCapabilityID      = "ui.settings-section"
	UISettingsSectionCapabilityVersion = "1.0.0"
)

// CapabilityRequirement is a package's independently versioned request for a
// host capability. Core package API v2 remains unchanged when capabilities are
// added or upgraded.
type CapabilityRequirement struct {
	Version     string          `json:"version"`
	Optional    bool            `json:"optional,omitempty"`
	Permissions json.RawMessage `json:"permissions,omitempty"`
}

// GrantedCapability is the concrete version negotiated by the current host.
type GrantedCapability struct {
	Version     string          `json:"version"`
	Permissions json.RawMessage `json:"permissions,omitempty"`
}

type CapabilityDescriptor struct {
	DescriptorVersion int                          `json:"descriptorVersion"`
	ID                string                       `json:"id"`
	Version           string                       `json:"version"`
	Summary           string                       `json:"summary"`
	Usage             CapabilityUsageDescriptor    `json:"usage"`
	Manifest          CapabilityManifestDescriptor `json:"manifest"`
	Runtime           CapabilityRuntimeDescriptor  `json:"runtime"`
}

type CapabilityUsageDescriptor struct {
	UseWhen         []string `json:"useWhen"`
	Constraints     []string `json:"constraints"`
	ManifestExample string   `json:"manifestExample"`
	RuntimeExample  string   `json:"runtimeExample"`
}

type CapabilityManifestDescriptor struct {
	RequirementJSONPointer string                      `json:"requirementJSONPointer"`
	Fields                 []CapabilityFieldDescriptor `json:"fields"`
}

type CapabilityRuntimeDescriptor struct {
	Scope          string                       `json:"scope"`
	RequiredAccess string                       `json:"requiredAccess"`
	OptionalAccess string                       `json:"optionalAccess"`
	Properties     []CapabilityFieldDescriptor  `json:"properties"`
	Methods        []CapabilityMethodDescriptor `json:"methods"`
}

type CapabilityMethodDescriptor struct {
	Name        string                      `json:"name"`
	Async       bool                        `json:"async"`
	Signature   string                      `json:"signature"`
	Description string                      `json:"description"`
	Inputs      []CapabilityFieldDescriptor `json:"inputs"`
	Outputs     []CapabilityFieldDescriptor `json:"outputs"`
	Errors      []CapabilityErrorDescriptor `json:"errors"`
}

type CapabilityFieldDescriptor struct {
	Path          string   `json:"path"`
	Type          string   `json:"type"`
	ItemType      string   `json:"itemType,omitempty"`
	Format        string   `json:"format,omitempty"`
	Required      bool     `json:"required"`
	Description   string   `json:"description"`
	Values        []string `json:"values,omitempty"`
	DefaultJSON   *string  `json:"defaultJSON,omitempty"`
	Minimum       *int     `json:"minimum,omitempty"`
	Maximum       *int     `json:"maximum,omitempty"`
	MinimumItems  *int     `json:"minimumItems,omitempty"`
	MaximumItems  *int     `json:"maximumItems,omitempty"`
	MinimumLength *int     `json:"minimumLength,omitempty"`
	MaximumLength *int     `json:"maximumLength,omitempty"`
}

type CapabilityErrorDescriptor struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type CapabilityInvocationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type capabilityDefinition struct {
	descriptor CapabilityDescriptor
	normalize  func(json.RawMessage) (json.RawMessage, error)
}

var capabilityDefinitions = map[string]capabilityDefinition{
	NetworkCapabilityID: {
		descriptor: networkCapabilityDescriptor(),
		normalize:  normalizeNetworkCapabilityPermissions,
	},
	UISettingsSectionCapabilityID: {
		descriptor: uiSettingsSectionCapabilityDescriptor(),
		normalize:  normalizeUISettingsSectionPermissions,
	},
}

func capabilityString(value string) *string { return &value }
func capabilityInt(value int) *int          { return &value }

func AvailableCapabilities() []CapabilityDescriptor {
	ids := make([]string, 0, len(capabilityDefinitions))
	for id := range capabilityDefinitions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]CapabilityDescriptor, 0, len(ids))
	for _, id := range ids {
		result = append(result, cloneCapabilityDescriptor(capabilityDefinitions[id].descriptor))
	}
	return result
}

func cloneCapabilityDescriptor(source CapabilityDescriptor) CapabilityDescriptor {
	result := source
	result.Usage.UseWhen = append([]string(nil), source.Usage.UseWhen...)
	result.Usage.Constraints = append([]string(nil), source.Usage.Constraints...)
	result.Manifest.Fields = cloneCapabilityFields(source.Manifest.Fields)
	result.Runtime.Properties = cloneCapabilityFields(source.Runtime.Properties)
	result.Runtime.Methods = make([]CapabilityMethodDescriptor, len(source.Runtime.Methods))
	for index, method := range source.Runtime.Methods {
		result.Runtime.Methods[index] = method
		result.Runtime.Methods[index].Inputs = cloneCapabilityFields(method.Inputs)
		result.Runtime.Methods[index].Outputs = cloneCapabilityFields(method.Outputs)
		result.Runtime.Methods[index].Errors = cloneCapabilityErrors(method.Errors)
	}
	return result
}

func cloneCapabilityFields(source []CapabilityFieldDescriptor) []CapabilityFieldDescriptor {
	result := make([]CapabilityFieldDescriptor, len(source))
	for index, field := range source {
		result[index] = field
		result[index].Values = append([]string(nil), field.Values...)
		result[index].DefaultJSON = cloneStringPointer(field.DefaultJSON)
		result[index].Minimum = cloneIntPointer(field.Minimum)
		result[index].Maximum = cloneIntPointer(field.Maximum)
		result[index].MinimumItems = cloneIntPointer(field.MinimumItems)
		result[index].MaximumItems = cloneIntPointer(field.MaximumItems)
		result[index].MinimumLength = cloneIntPointer(field.MinimumLength)
		result[index].MaximumLength = cloneIntPointer(field.MaximumLength)
	}
	return result
}

func cloneCapabilityErrors(source []CapabilityErrorDescriptor) []CapabilityErrorDescriptor {
	if source == nil {
		return nil
	}
	result := make([]CapabilityErrorDescriptor, len(source))
	copy(result, source)
	return result
}

func cloneIntPointer(source *int) *int {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func ResolveCapabilityRequirements(
	requirements map[string]CapabilityRequirement,
) (map[string]GrantedCapability, error) {
	grants := map[string]GrantedCapability{}
	ids := make([]string, 0, len(requirements))
	for id := range requirements {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		requirement := requirements[id]
		if strings.TrimSpace(id) == "" {
			return nil, errors.New("能力标识不能为空")
		}
		version, ok := ParseVersionRequirement(requirement.Version)
		if !ok {
			return nil, fmt.Errorf("能力 %s 的版本要求无效：%s", id, requirement.Version)
		}
		definition, exists := capabilityDefinitions[id]
		if !exists {
			if requirement.Optional {
				continue
			}
			return nil, fmt.Errorf("当前 Codex Tweaks 不提供能力 %s", id)
		}
		if !version.Contains(definition.descriptor.Version) {
			if requirement.Optional {
				continue
			}
			return nil, fmt.Errorf(
				"能力 %s 需要 %s，当前提供 %s",
				id, requirement.Version, definition.descriptor.Version,
			)
		}
		permissions, err := definition.normalize(requirement.Permissions)
		if err != nil {
			return nil, fmt.Errorf("能力 %s 的权限无效：%w", id, err)
		}
		grants[id] = GrantedCapability{
			Version: definition.descriptor.Version, Permissions: permissions,
		}
	}
	return grants, nil
}

func cloneCapabilityRequirements(
	source map[string]CapabilityRequirement,
) map[string]CapabilityRequirement {
	result := make(map[string]CapabilityRequirement, len(source))
	for id, requirement := range source {
		requirement.Permissions = append(json.RawMessage(nil), requirement.Permissions...)
		result[id] = requirement
	}
	return result
}

func resolvePackageCapabilities(
	packageID string,
	requirements map[string]CapabilityRequirement,
) (map[string]GrantedCapability, error) {
	grants, err := ResolveCapabilityRequirements(requirements)
	if err != nil {
		return nil, err
	}
	if grant, ok := grants[UISettingsSectionCapabilityID]; ok {
		permissions, err := bindUISettingsSectionPermissions(packageID, grant.Permissions)
		if err != nil {
			return nil, err
		}
		grant.Permissions = permissions
		grants[UISettingsSectionCapabilityID] = grant
	}
	return grants, nil
}

type CapabilityBroker struct {
	network *networkCapability
}

func NewCapabilityBroker() *CapabilityBroker {
	return &CapabilityBroker{network: newNetworkCapability()}
}

func (b *CapabilityBroker) Invoke(
	ctx context.Context,
	grants map[string]GrantedCapability,
	capabilityID string,
	method string,
	parameters json.RawMessage,
) (any, *CapabilityInvocationError) {
	grant, ok := grants[capabilityID]
	if !ok {
		return nil, capabilityFailure("permission_denied", "Capability was not granted to this package")
	}
	switch capabilityID {
	case NetworkCapabilityID:
		return b.network.Invoke(ctx, grant, method, parameters)
	case UISettingsSectionCapabilityID:
		return nil, capabilityFailure("unsupported_method", "Settings sections are managed inside the page runtime")
	default:
		return nil, capabilityFailure("capability_unavailable", "Capability is not available")
	}
}

func capabilityFailure(code, message string) *CapabilityInvocationError {
	return &CapabilityInvocationError{Code: code, Message: message}
}

func decodeCapabilityJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("只能包含一个 JSON 值")
		}
		return err
	}
	return nil
}
