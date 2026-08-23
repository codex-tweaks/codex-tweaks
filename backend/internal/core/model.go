package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

const (
	APIVersion      = 2
	CompilerVersion = "0.25.9"
)

type RemoteSelectorType string

const (
	SelectorBranch              RemoteSelectorType = "branch"
	SelectorLatestSemverTag     RemoteSelectorType = "latestSemverTag"
	SelectorTag                 RemoteSelectorType = "tag"
	SelectorGitHubLatestRelease RemoteSelectorType = "githubLatestRelease"
	SelectorGitHubRelease       RemoteSelectorType = "githubRelease"
	SelectorCommit              RemoteSelectorType = "commit"
)

type RemoteSelector struct {
	Type  RemoteSelectorType `json:"type"`
	Value *string            `json:"value,omitempty"`
}

func NewRemoteSelector(selectorType RemoteSelectorType, rawValue string) RemoteSelector {
	selector := RemoteSelector{Type: selectorType}
	if value := strings.TrimSpace(rawValue); value != "" {
		selector.Value = &value
	}
	return selector
}

func (s RemoteSelector) IsPinned() bool {
	switch s.Type {
	case SelectorTag, SelectorGitHubRelease, SelectorCommit:
		return true
	default:
		return false
	}
}

type PackageSource struct {
	URL      string         `json:"url"`
	Selector RemoteSelector `json:"selector"`
}

type PackageDependency struct {
	Version string         `json:"version"`
	Source  *PackageSource `json:"source,omitempty"`
}

type PackageConfiguration struct {
	APIVersion          int                          `json:"apiVersion"`
	Entry               string                       `json:"entry"`
	Priority            int                          `json:"priority"`
	PackageDependencies map[string]PackageDependency `json:"packageDependencies"`
}

func (c *PackageConfiguration) UnmarshalJSON(data []byte) error {
	type configuration PackageConfiguration
	value := configuration{APIVersion: APIVersion, PackageDependencies: map[string]PackageDependency{}}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.PackageDependencies == nil {
		value.PackageDependencies = map[string]PackageDependency{}
	}
	*c = PackageConfiguration(value)
	return nil
}

type PackageManifest struct {
	Name         string               `json:"name"`
	Version      string               `json:"version"`
	Description  string               `json:"description"`
	Type         *string              `json:"type,omitempty"`
	Dependencies map[string]string    `json:"dependencies"`
	CodexTweaks  PackageConfiguration `json:"codexTweaks"`
}

func (m *PackageManifest) UnmarshalJSON(data []byte) error {
	type manifest PackageManifest
	value := manifest{Dependencies: map[string]string{}}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.Dependencies == nil {
		value.Dependencies = map[string]string{}
	}
	*m = PackageManifest(value)
	return nil
}

type CodableTime struct{ time.Time }

func NewCodableTime(value time.Time) CodableTime { return CodableTime{Time: value} }

func (t CodableTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.UTC().Format(time.RFC3339Nano))
}

func (t *CodableTime) UnmarshalJSON(data []byte) error {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, encoded)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

type PackageBuildRecord struct {
	PackageID             string            `json:"packageID"`
	PackageVersion        string            `json:"packageVersion"`
	PackageDependencies   map[string]string `json:"packageDependencies"`
	SourceFingerprint     string            `json:"sourceFingerprint"`
	DependencyFingerprint string            `json:"dependencyFingerprint"`
	CompilerVersion       string            `json:"compilerVersion"`
	NodeVersion           string            `json:"nodeVersion"`
	BuildDirectoryName    string            `json:"buildDirectoryName"`
	HasCSS                bool              `json:"hasCSS"`
	BuiltAt               CodableTime       `json:"builtAt"`
}

func (r *PackageBuildRecord) UnmarshalJSON(data []byte) error {
	type record PackageBuildRecord
	value := record{PackageDependencies: map[string]string{}}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.PackageDependencies == nil {
		value.PackageDependencies = map[string]string{}
	}
	*r = PackageBuildRecord(value)
	return nil
}

type ActivePackageBuild struct {
	Record          PackageBuildRecord `json:"record"`
	OutputDirectory string             `json:"outputDirectory"`
}

func (b ActivePackageBuild) JavaScriptPath() string { return joinPath(b.OutputDirectory, "bundle.js") }
func (b ActivePackageBuild) CSSPath() string        { return joinPath(b.OutputDirectory, "bundle.css") }

type ManagedPackageRegistration struct {
	PackageID             string        `json:"packageID"`
	Source                PackageSource `json:"source"`
	AddedAt               CodableTime   `json:"addedAt"`
	VersionRequirements   []string      `json:"versionRequirements"`
	LastCheckedAt         *CodableTime  `json:"lastCheckedAt,omitempty"`
	RemoteETag            *string       `json:"remoteETag,omitempty"`
	LastResolvedReference *string       `json:"lastResolvedReference,omitempty"`
	LastResolvedCommit    *string       `json:"lastResolvedCommit,omitempty"`
}

type ManagedPackageRegistry struct {
	SchemaVersion int                                   `json:"schemaVersion"`
	Packages      map[string]ManagedPackageRegistration `json:"packages"`
}

func NewManagedPackageRegistry() ManagedPackageRegistry {
	return ManagedPackageRegistry{SchemaVersion: 1, Packages: map[string]ManagedPackageRegistration{}}
}

type ManagedPackageLock struct {
	PackageID          string        `json:"packageID"`
	PackageVersion     string        `json:"packageVersion"`
	Source             PackageSource `json:"source"`
	ResolvedReference  string        `json:"resolvedReference"`
	ResolvedCommit     string        `json:"resolvedCommit"`
	SourceRelativePath string        `json:"sourceRelativePath"`
	InstalledAt        CodableTime   `json:"installedAt"`
}

type ManagedPackageLockfile struct {
	SchemaVersion int                           `json:"schemaVersion"`
	Packages      map[string]ManagedPackageLock `json:"packages"`
}

func NewManagedPackageLockfile() ManagedPackageLockfile {
	return ManagedPackageLockfile{SchemaVersion: 1, Packages: map[string]ManagedPackageLock{}}
}

type PackageOriginKind string

const (
	OriginLocal   PackageOriginKind = "local"
	OriginManaged PackageOriginKind = "managed"
)

type PackageOrigin struct {
	Kind PackageOriginKind   `json:"kind"`
	Lock *ManagedPackageLock `json:"lock,omitempty"`
}

func LocalOrigin() PackageOrigin { return PackageOrigin{Kind: OriginLocal} }

func ManagedOrigin(lock ManagedPackageLock) PackageOrigin {
	return PackageOrigin{Kind: OriginManaged, Lock: &lock}
}

type PackageUserSetting struct {
	PriorityOverride *int `json:"priorityOverride,omitempty"`
}

type PackageUserSettings struct {
	SchemaVersion int                           `json:"schemaVersion"`
	Packages      map[string]PackageUserSetting `json:"packages"`
}

func NewPackageUserSettings() PackageUserSettings {
	return PackageUserSettings{SchemaVersion: 1, Packages: map[string]PackageUserSetting{}}
}

type BuildDisposition string

const (
	BuildInvalid          BuildDisposition = "invalid"
	BuildNotBuilt         BuildDisposition = "notBuilt"
	BuildCurrent          BuildDisposition = "current"
	BuildVersionUpdate    BuildDisposition = "versionUpdate"
	BuildDependencyUpdate BuildDisposition = "dependencyUpdate"
	BuildSourceChanged    BuildDisposition = "sourceChanged"
	BuildCompilerUpdate   BuildDisposition = "compilerUpdate"
)

type Package struct {
	ID                    string              `json:"id"`
	DirectoryName         string              `json:"directoryName"`
	Directory             string              `json:"directory"`
	Manifest              *PackageManifest    `json:"manifest,omitempty"`
	SourceFingerprint     *string             `json:"sourceFingerprint,omitempty"`
	DependencyFingerprint *string             `json:"dependencyFingerprint,omitempty"`
	ActiveBuild           *ActivePackageBuild `json:"activeBuild,omitempty"`
	ValidationError       *string             `json:"validationError,omitempty"`
	PriorityOverride      *int                `json:"priorityOverride,omitempty"`
	Origin                PackageOrigin       `json:"origin"`
}

func (p Package) DisplayName() string {
	if p.Manifest != nil {
		return p.Manifest.Name
	}
	return p.DirectoryName
}

func (p Package) Version() string {
	if p.Manifest != nil {
		return p.Manifest.Version
	}
	return "—"
}

func (p Package) DeclaredPriority() int {
	if p.Manifest == nil {
		return 0
	}
	return p.Manifest.CodexTweaks.Priority
}

func (p Package) Priority() int {
	if p.PriorityOverride != nil {
		return *p.PriorityOverride
	}
	return p.DeclaredPriority()
}

func (p Package) PackageDependencies() map[string]PackageDependency {
	if p.Manifest == nil {
		return map[string]PackageDependency{}
	}
	return p.Manifest.CodexTweaks.PackageDependencies
}

func (p Package) RuntimePackageDependencies() map[string]string {
	if p.ActiveBuild != nil {
		return cloneStringMap(p.ActiveBuild.Record.PackageDependencies)
	}
	result := map[string]string{}
	for packageID, dependency := range p.PackageDependencies() {
		result[packageID] = dependency.Version
	}
	return result
}

func (p Package) ManagedLock() *ManagedPackageLock {
	if p.Origin.Kind == OriginManaged {
		return p.Origin.Lock
	}
	return nil
}

func (p Package) BuildDisposition(compilerVersion string) BuildDisposition {
	if p.ValidationError != nil || p.Manifest == nil || p.SourceFingerprint == nil || p.DependencyFingerprint == nil {
		return BuildInvalid
	}
	if p.ActiveBuild == nil {
		return BuildNotBuilt
	}
	record := p.ActiveBuild.Record
	if record.PackageVersion != p.Manifest.Version {
		return BuildVersionUpdate
	}
	if record.DependencyFingerprint != *p.DependencyFingerprint {
		return BuildDependencyUpdate
	}
	if record.SourceFingerprint != *p.SourceFingerprint {
		return BuildSourceChanged
	}
	if record.CompilerVersion != compilerVersion {
		return BuildCompilerUpdate
	}
	return BuildCurrent
}

func (p Package) BuildRequestKey(compilerVersion string) (string, bool) {
	if p.ValidationError != nil || p.Manifest == nil || p.SourceFingerprint == nil || p.DependencyFingerprint == nil {
		return "", false
	}
	return strings.Join([]string{p.Manifest.Version, *p.SourceFingerprint, *p.DependencyFingerprint, compilerVersion}, "\x00"), true
}

type EnablementReconciliation struct {
	KnownPackageIDs           map[string]bool `json:"knownPackageIDs"`
	DisabledPackageIDs        map[string]bool `json:"disabledPackageIDs"`
	NewlyDiscoveredPackageIDs map[string]bool `json:"newlyDiscoveredPackageIDs"`
}

func ReconcileEnablement(discovered, known, disabled map[string]bool, hasKnownBaseline bool) EnablementReconciliation {
	if !hasKnownBaseline {
		return EnablementReconciliation{
			KnownPackageIDs:           cloneSet(discovered),
			DisabledPackageIDs:        cloneSet(disabled),
			NewlyDiscoveredPackageIDs: map[string]bool{},
		}
	}
	newlyDiscovered := map[string]bool{}
	resultKnown := cloneSet(known)
	resultDisabled := cloneSet(disabled)
	for packageID := range discovered {
		if !known[packageID] {
			newlyDiscovered[packageID] = true
			resultDisabled[packageID] = true
		}
		resultKnown[packageID] = true
	}
	return EnablementReconciliation{
		KnownPackageIDs:           resultKnown,
		DisabledPackageIDs:        resultDisabled,
		NewlyDiscoveredPackageIDs: newlyDiscovered,
	}
}

type CompiledPackage struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	BuildFingerprint string   `json:"buildFingerprint"`
	DependencyIDs    []string `json:"dependencyIDs"`
	CSS              string   `json:"css"`
	JavaScript       string   `json:"javascript"`
}

type Payload struct {
	Packages []CompiledPackage `json:"packages"`
	Version  string            `json:"version"`
}

type PayloadLoadResult struct {
	Payload       Payload           `json:"payload"`
	PackageErrors map[string]string `json:"packageErrors"`
}

func FingerprintString(value string) string { return FingerprintBytes([]byte(value)) }

func FingerprintBytes(value []byte) string {
	hash := fnv.New64a()
	_, _ = hash.Write(value)
	return fmt.Sprintf("%016x", hash.Sum64())
}

func JSONLiteral(value any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		panic(err)
	}
	return strings.TrimSuffix(buffer.String(), "\n")
}

func SourcesMatch(left, right PackageSource) bool {
	return normalizedRepositoryURL(left.URL) == normalizedRepositoryURL(right.URL) && selectorsEqual(left.Selector, right.Selector)
}

func normalizedRepositoryURL(raw string) string {
	value := strings.ToLower(strings.Trim(strings.TrimSpace(raw), "/"))
	return strings.TrimSuffix(value, ".git")
}

func selectorsEqual(left, right RemoteSelector) bool {
	if left.Type != right.Type {
		return false
	}
	if left.Value == nil || right.Value == nil {
		return left.Value == nil && right.Value == nil
	}
	return *left.Value == *right.Value
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneSet(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for key, value := range source {
		if value {
			result[key] = true
		}
	}
	return result
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
