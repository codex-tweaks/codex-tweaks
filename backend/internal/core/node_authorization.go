package core

import (
	"errors"
	"os"
	"sort"
	"time"
)

const nodeAuthorizationSchemaVersion = 1

type NodeAuthorizationRecord struct {
	AuthorizationID string      `json:"authorizationID"`
	AuthorizedAt    CodableTime `json:"authorizedAt"`
}

type NodeAuthorizationState struct {
	SchemaVersion int                                `json:"schemaVersion"`
	Packages      map[string]NodeAuthorizationRecord `json:"packages"`
}

func NodeAuthorizationID(pkg Package) string {
	if pkg.Manifest == nil || pkg.ActiveBuild == nil || pkg.BuildDisposition(CompilerVersion) != BuildCurrent {
		return ""
	}
	record := pkg.ActiveBuild.Record
	if !record.HasNode || record.NodePermission == nil || record.Entrypoints.Node == nil || record.NodeBundleFingerprint == "" {
		return ""
	}
	resolvedCommit := ""
	if lock := pkg.ManagedLock(); lock != nil {
		resolvedCommit = lock.ResolvedCommit
	}
	return SecureFingerprintStrings(
		pkg.ID,
		record.PackageVersion,
		record.ManifestFingerprint,
		record.SourceFingerprint,
		record.DependencyFingerprint,
		record.RendererFingerprint,
		record.CSSFingerprint,
		record.NodeBundleFingerprint,
		record.CompilerVersion,
		*record.Entrypoints.Node,
		record.NodePermission.Reason,
		resolvedCommit,
	)
}

func (s *Store) LoadNodeAuthorizations() (map[string]NodeAuthorizationRecord, error) {
	state := NodeAuthorizationState{}
	if _, err := os.Stat(s.NodeAuthorizationsPath); errors.Is(err, os.ErrNotExist) {
		return map[string]NodeAuthorizationRecord{}, nil
	} else if err != nil {
		return nil, err
	}
	if err := readJSON(s.NodeAuthorizationsPath, &state); err != nil {
		return nil, err
	}
	if state.SchemaVersion != nodeAuthorizationSchemaVersion {
		return nil, errors.New("不支持的 Node 授权状态版本。")
	}
	if state.Packages == nil {
		state.Packages = map[string]NodeAuthorizationRecord{}
	}
	return state.Packages, nil
}

func (s *Store) SaveNodeAuthorizations(records map[string]NodeAuthorizationRecord) error {
	copyRecords := make(map[string]NodeAuthorizationRecord, len(records))
	for packageID, record := range records {
		if packageID != "" && record.AuthorizationID != "" {
			copyRecords[packageID] = record
		}
	}
	return writeJSONAtomic(s.NodeAuthorizationsPath, NodeAuthorizationState{
		SchemaVersion: nodeAuthorizationSchemaVersion,
		Packages:      copyRecords,
	})
}

func authorizeNodeRecord(records map[string]NodeAuthorizationRecord, packageID, authorizationID string) {
	records[packageID] = NodeAuthorizationRecord{
		AuthorizationID: authorizationID,
		AuthorizedAt:    NewCodableTime(time.Now()),
	}
}

func currentExplicitNodeAuthorizations(packages []Package, records map[string]NodeAuthorizationRecord) map[string]bool {
	result := map[string]bool{}
	for _, pkg := range packages {
		if authorizationID := NodeAuthorizationID(pkg); authorizationID != "" && records[pkg.ID].AuthorizationID == authorizationID {
			result[pkg.ID] = true
		}
	}
	return result
}

func (c *Controller) nodeTrustModeLocked(pkg Package) string {
	authorizationID := NodeAuthorizationID(pkg)
	if authorizationID == "" {
		return ""
	}
	if c.nodeAuthorizations[pkg.ID].AuthorizationID == authorizationID {
		return NodeTrustExplicit
	}
	if c.config.DeveloperMode && c.developerAllowUnknownNode {
		return NodeTrustAutomatic
	}
	return ""
}

func (c *Controller) nodeTrustByPackageIDLocked() map[string]string {
	result := map[string]string{}
	for _, pkg := range c.packages {
		if mode := c.nodeTrustModeLocked(pkg); mode != "" {
			result[pkg.ID] = mode
		}
	}
	return result
}

func (c *Controller) enabledNodeEnvironmentLocked() *NodeEnvironment {
	if !c.config.Enabled {
		return nil
	}
	return cloneNodeEnvironment(c.nodeEnvironment)
}

func (c *Controller) clearUnauthorizedNodeRuntimeErrorsLocked() {
	for _, pkg := range c.packages {
		if pkg.Manifest != nil && pkg.Manifest.CodexTweaks.Permissions.Node != nil && c.nodeTrustModeLocked(pkg) == "" {
			delete(c.packageRuntimeErrors, pkg.ID)
		}
	}
}

func (c *Controller) packageNodeViewLocked(pkg Package) *PackageNodeView {
	if pkg.Manifest == nil || pkg.Manifest.CodexTweaks.Permissions.Node == nil {
		return nil
	}
	authorizationID := NodeAuthorizationID(pkg)
	mode := c.nodeTrustModeLocked(pkg)
	status := "pendingAuthorization"
	if authorizationID == "" {
		status = "revisionPending"
	} else if mode == NodeTrustExplicit {
		status = "explicitlyAuthorized"
	} else if mode == NodeTrustAutomatic {
		status = "developerAutomatic"
	}
	running := false
	if c.nodeRuntime != nil {
		running = c.nodeRuntime.RunningPackageIDs()[pkg.ID]
	}
	return &PackageNodeView{
		AuthorizationID:      authorizationID,
		Reason:               pkg.Manifest.CodexTweaks.Permissions.Node.Reason,
		Status:               status,
		Authorized:           mode != "",
		ExplicitlyAuthorized: mode == NodeTrustExplicit,
		AutomaticallyAllowed: mode == NodeTrustAutomatic,
		Running:              running,
	}
}

func sortedAuthorizationPackageIDs(records map[string]NodeAuthorizationRecord) []string {
	result := make([]string, 0, len(records))
	for packageID := range records {
		result = append(result, packageID)
	}
	sort.Strings(result)
	return result
}
