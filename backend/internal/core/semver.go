package core

import (
	"strings"
	"unicode"
)

// SemanticVersion implements SemVer 2.0.0 precedence without converting
// numeric identifiers to machine integers.
type SemanticVersion struct {
	Major      string
	Minor      string
	Patch      string
	Prerelease []prereleaseIdentifier
	HasPre     bool
}

type prereleaseIdentifier struct {
	Value   string
	Numeric bool
}

func NormalizeVersion(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) > 0 && (value[0] == 'v' || value[0] == 'V') {
		return value[1:]
	}
	return value
}

func ParseSemanticVersion(raw string) (SemanticVersion, bool) {
	value := NormalizeVersion(raw)
	buildParts := strings.SplitN(value, "+", 2)
	if buildParts[0] == "" || (len(buildParts) == 2 && !validDotIdentifiers(buildParts[1])) {
		return SemanticVersion{}, false
	}

	precedence := strings.SplitN(buildParts[0], "-", 2)
	core := strings.Split(precedence[0], ".")
	if len(core) != 3 || !validNumeric(core[0]) || !validNumeric(core[1]) || !validNumeric(core[2]) {
		return SemanticVersion{}, false
	}

	version := SemanticVersion{Major: core[0], Minor: core[1], Patch: core[2]}
	if len(precedence) == 1 {
		return version, true
	}

	parts := strings.Split(precedence[1], ".")
	if len(parts) == 0 {
		return SemanticVersion{}, false
	}
	version.HasPre = true
	for _, part := range parts {
		if part == "" || !validIdentifier(part) {
			return SemanticVersion{}, false
		}
		numeric := asciiDigits(part)
		if numeric && !validNumeric(part) {
			return SemanticVersion{}, false
		}
		version.Prerelease = append(version.Prerelease, prereleaseIdentifier{Value: part, Numeric: numeric})
	}
	return version, true
}

func (v SemanticVersion) Compare(other SemanticVersion) int {
	for _, pair := range [][2]string{{v.Major, other.Major}, {v.Minor, other.Minor}, {v.Patch, other.Patch}} {
		if result := compareNumeric(pair[0], pair[1]); result != 0 {
			return result
		}
	}
	if !v.HasPre && !other.HasPre {
		return 0
	}
	if !v.HasPre {
		return 1
	}
	if !other.HasPre {
		return -1
	}
	limit := min(len(v.Prerelease), len(other.Prerelease))
	for i := 0; i < limit; i++ {
		left, right := v.Prerelease[i], other.Prerelease[i]
		if left == right {
			continue
		}
		switch {
		case left.Numeric && right.Numeric:
			return compareNumeric(left.Value, right.Value)
		case left.Numeric:
			return -1
		case right.Numeric:
			return 1
		case left.Value < right.Value:
			return -1
		default:
			return 1
		}
	}
	if len(v.Prerelease) < len(other.Prerelease) {
		return -1
	}
	if len(v.Prerelease) > len(other.Prerelease) {
		return 1
	}
	return 0
}

func (v SemanticVersion) Stable() bool { return !v.HasPre }

func (v SemanticVersion) BetaOrRC() bool {
	if !v.HasPre || len(v.Prerelease) == 0 || v.Prerelease[0].Numeric {
		return false
	}
	value := strings.ToLower(v.Prerelease[0].Value)
	return value == "beta" || value == "rc"
}

type versionRule struct {
	Any          bool
	Exact        *SemanticVersion
	Lower        *SemanticVersion
	IncludeLower bool
	Upper        *SemanticVersion
	IncludeUpper bool
}

type SemanticVersionRequirement struct {
	Raw  string
	rule versionRule
}

func ParseVersionRequirement(raw string) (SemanticVersionRequirement, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return SemanticVersionRequirement{}, false
	}
	requirement := SemanticVersionRequirement{Raw: value}
	if value == "*" || strings.EqualFold(value, "latest") {
		requirement.rule.Any = true
		return requirement, true
	}

	for _, prefix := range []string{">=", "<=", ">", "<"} {
		if !strings.HasPrefix(value, prefix) {
			continue
		}
		version, ok := ParseSemanticVersion(strings.TrimSpace(strings.TrimPrefix(value, prefix)))
		if !ok {
			return SemanticVersionRequirement{}, false
		}
		switch prefix {
		case ">=":
			requirement.rule.Lower, requirement.rule.IncludeLower = &version, true
		case ">":
			requirement.rule.Lower = &version
		case "<=":
			requirement.rule.Upper, requirement.rule.IncludeUpper = &version, true
		case "<":
			requirement.rule.Upper = &version
		}
		return requirement, true
	}

	if strings.HasPrefix(value, "^") || strings.HasPrefix(value, "~") {
		prefix := value[0]
		lower, ok := ParseSemanticVersion(value[1:])
		if !ok {
			return SemanticVersionRequirement{}, false
		}
		major, minor, patch := decimalIncrementInputs(lower)
		var upperText string
		if prefix == '~' {
			upperText = major + "." + incrementDecimal(minor) + ".0"
		} else if major != "0" {
			upperText = incrementDecimal(major) + ".0.0"
		} else if minor != "0" {
			upperText = "0." + incrementDecimal(minor) + ".0"
		} else {
			upperText = "0.0." + incrementDecimal(patch)
		}
		upper, _ := ParseSemanticVersion(upperText)
		requirement.rule.Lower, requirement.rule.IncludeLower = &lower, true
		requirement.rule.Upper = &upper
		return requirement, true
	}

	if strings.ContainsAny(strings.ToLower(value), "x*") {
		rule, ok := wildcardRule(value)
		if !ok {
			return SemanticVersionRequirement{}, false
		}
		requirement.rule = rule
		return requirement, true
	}

	if exact, ok := ParseSemanticVersion(value); ok {
		requirement.rule.Exact = &exact
		return requirement, true
	}

	parts := strings.Split(value, ".")
	if len(parts) == 1 && validNumeric(parts[0]) {
		lower, _ := ParseSemanticVersion(parts[0] + ".0.0")
		upper, _ := ParseSemanticVersion(incrementDecimal(parts[0]) + ".0.0")
		requirement.rule.Lower, requirement.rule.IncludeLower, requirement.rule.Upper = &lower, true, &upper
		return requirement, true
	}
	if len(parts) == 2 && validNumeric(parts[0]) && validNumeric(parts[1]) {
		lower, _ := ParseSemanticVersion(parts[0] + "." + parts[1] + ".0")
		upper, _ := ParseSemanticVersion(parts[0] + "." + incrementDecimal(parts[1]) + ".0")
		requirement.rule.Lower, requirement.rule.IncludeLower, requirement.rule.Upper = &lower, true, &upper
		return requirement, true
	}
	return SemanticVersionRequirement{}, false
}

func (r SemanticVersionRequirement) Contains(raw string) bool {
	version, ok := ParseSemanticVersion(raw)
	if !ok {
		return false
	}
	if r.rule.Any {
		return true
	}
	if r.rule.Exact != nil {
		return version.Compare(*r.rule.Exact) == 0
	}
	if r.rule.Lower != nil {
		comparison := version.Compare(*r.rule.Lower)
		if comparison < 0 || (!r.rule.IncludeLower && comparison == 0) {
			return false
		}
	}
	if r.rule.Upper != nil {
		comparison := version.Compare(*r.rule.Upper)
		if comparison > 0 || (!r.rule.IncludeUpper && comparison == 0) {
			return false
		}
	}
	return true
}

func wildcardRule(value string) (versionRule, bool) {
	parts := strings.Split(strings.ToLower(value), ".")
	if len(parts) < 1 || len(parts) > 3 {
		return versionRule{}, false
	}
	wild := func(value string) bool { return value == "x" || value == "*" }
	if wild(parts[0]) {
		return versionRule{Any: true}, true
	}
	if !validNumeric(parts[0]) {
		return versionRule{}, false
	}
	if len(parts) == 1 || wild(parts[1]) {
		lower, _ := ParseSemanticVersion(parts[0] + ".0.0")
		upper, _ := ParseSemanticVersion(incrementDecimal(parts[0]) + ".0.0")
		return versionRule{Lower: &lower, IncludeLower: true, Upper: &upper}, true
	}
	if !validNumeric(parts[1]) {
		return versionRule{}, false
	}
	if len(parts) == 2 || wild(parts[2]) {
		lower, _ := ParseSemanticVersion(parts[0] + "." + parts[1] + ".0")
		upper, _ := ParseSemanticVersion(parts[0] + "." + incrementDecimal(parts[1]) + ".0")
		return versionRule{Lower: &lower, IncludeLower: true, Upper: &upper}, true
	}
	return versionRule{}, false
}

func decimalIncrementInputs(v SemanticVersion) (string, string, string) {
	return v.Major, v.Minor, v.Patch
}

func incrementDecimal(value string) string {
	bytes := []byte(value)
	carry := byte(1)
	for i := len(bytes) - 1; i >= 0 && carry == 1; i-- {
		if bytes[i] == '9' {
			bytes[i] = '0'
		} else {
			bytes[i]++
			carry = 0
		}
	}
	if carry == 1 {
		return "1" + string(bytes)
	}
	return string(bytes)
}

func compareNumeric(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func validNumeric(value string) bool {
	return asciiDigits(value) && !(len(value) > 1 && value[0] == '0')
}

func asciiDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(unicode.Is(unicode.ASCII_Hex_Digit, r) || (r >= 'g' && r <= 'z') || (r >= 'G' && r <= 'Z') || r == '-') {
			return false
		}
	}
	return true
}

func validDotIdentifiers(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if !validIdentifier(part) {
			return false
		}
	}
	return true
}
