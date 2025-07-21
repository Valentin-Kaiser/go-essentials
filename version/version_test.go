package version_test

import (
	"testing"

	"github.com/Valentin-Kaiser/go-core/version"
)

func TestGetVersion(t *testing.T) {
	t.Parallel()
	version := version.GetVersion()
	if version == nil {
		t.Error("GetVersion() returned nil")
		return
	}

	if version.GitTag == "" {
		t.Error("GitTag should not be empty")
	}

	if version.GitCommit == "" {
		t.Error("GitCommit should not be empty")
	}

	if version.GitShort == "" {
		t.Error("GitShort should not be empty")
	}

	if version.BuildDate == "" {
		t.Error("BuildDate should not be empty")
	}

	if version.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}

	if version.Platform == "" {
		t.Error("Platform should not be empty")
	}

	if version.Modules == nil {
		t.Error("Modules should not be nil")
	}
}

func TestMajor(t *testing.T) {
	t.Parallel()
	// Test with default tag
	originalTag := version.GitTag
	defer func() { version.GitTag = originalTag }()

	version.GitTag = "v1.2.3"
	major := version.Major()
	if major != 1 {
		t.Errorf("Expected major version 1, got %d", major)
	}

	// Test with invalid tag
	version.GitTag = "invalid"
	major = version.Major()
	if major != 0 {
		t.Errorf("Expected major version 0 for invalid tag, got %d", major)
	}
}

func TestMinor(t *testing.T) {
	t.Parallel()
	originalTag := version.GitTag
	defer func() { version.GitTag = originalTag }()

	version.GitTag = "v1.2.3"
	minor := version.Minor()
	if minor != 2 {
		t.Errorf("Expected minor version 2, got %d", minor)
	}

	// Test with invalid tag
	version.GitTag = "invalid"
	minor = version.Minor()
	if minor != 0 {
		t.Errorf("Expected minor version 0 for invalid tag, got %d", minor)
	}
}

func TestPatch(t *testing.T) {
	t.Parallel()
	originalTag := version.GitTag
	defer func() { version.GitTag = originalTag }()

	version.GitTag = "v1.2.3"
	patch := version.Patch()
	if patch != 3 {
		t.Errorf("Expected patch version 3, got %d", patch)
	}

	// Test with invalid tag
	version.GitTag = "invalid"
	patch = version.Patch()
	if patch != 0 {
		t.Errorf("Expected patch version 0 for invalid tag, got %d", patch)
	}
}

func TestString(t *testing.T) {
	t.Parallel()
	originalTag := version.GitTag
	defer func() { version.GitTag = originalTag }()

	version.GitTag = "v1.2.3"
	str := version.String()
	if str != "1.2.3" {
		t.Errorf("Expected version string '1.2.3', got '%s'", str)
	}

	// Test with pre-release tag
	version.GitTag = "v1.2.3-alpha"
	str = version.String()
	if str != "1.2.3" {
		t.Errorf("Expected version string '1.2.3' for pre-release, got '%s'", str)
	}
}

func TestIsGitTag(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		tag      string
		expected bool
	}{
		{"v1.2.3", true},
		{"v0.0.1", true},
		{"v10.20.30", true},
		{"1.2.3", false},
		{"v1.2", false},
		{"v1.2.3.4", true},
		{"invalid", false},
		{"", false},
	}

	for _, tc := range testCases {
		result := version.IsGitTag(tc.tag)
		if result != tc.expected {
			t.Errorf("IsGitTag(%q) = %v, expected %v", tc.tag, result, tc.expected)
		}
	}
}

func TestParseTagSegment(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		tag      string
		segment  int
		expected int
	}{
		{"v1.2.3", 0, 1},
		{"v1.2.3", 1, 2},
		{"v1.2.3", 2, 3},
		{"v1.2.3", 3, 0},  // out of range
		{"invalid", 0, 0}, // invalid tag
		{"v1.2", 2, 0},    // insufficient segments
	}

	for _, tc := range testCases {
		result := version.ParseTagSegment(tc.tag, tc.segment)
		if result != tc.expected {
			t.Errorf("ParseTagSegment(%q, %d) = %d, expected %d", tc.tag, tc.segment, result, tc.expected)
		}
	}
}

func TestParseTagVersion(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		tag      string
		expected string
	}{
		{"v1.2.3", "1.2.3"},
		{"v1.2.3-alpha", "1.2.3"},
		{"invalid", ""},
		{"", ""},
	}

	for _, tc := range testCases {
		result := version.ParseTagVersion(tc.tag)
		if result != tc.expected {
			t.Errorf("ParseTagVersion(%q) = %q, expected %q", tc.tag, result, tc.expected)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	t.Parallel()
	v1 := &version.Release{
		GitTag:    "v1.2.3",
		GitCommit: "abc123",
		GitShort:  "abc",
	}

	v2 := &version.Release{
		GitTag:    "v1.2.3",
		GitCommit: "abc123",
		GitShort:  "abc",
	}

	v3 := &version.Release{
		GitTag:    "v1.2.4",
		GitCommit: "def456",
		GitShort:  "def",
	}

	// Test identical versions
	if !v1.Compare(v2) {
		t.Error("Identical versions should be equal")
	}

	// Test different versions
	if v1.Compare(v3) {
		t.Error("Different versions should not be equal")
	}
}

func TestVersionCompareTag(t *testing.T) {
	t.Parallel()
	v1 := &version.Release{GitTag: "v1.2.3"}
	v2 := &version.Release{GitTag: "v1.2.3"}
	v3 := &version.Release{GitTag: "v1.2.4"}

	if !v1.CompareTag(v2) {
		t.Error("Identical tags should be equal")
	}

	if v1.CompareTag(v3) {
		t.Error("Different tags should not be equal")
	}
}

func TestVersionCompareCommit(t *testing.T) {
	t.Parallel()
	v1 := &version.Release{GitCommit: "abc123", GitShort: "abc"}
	v2 := &version.Release{GitCommit: "abc123", GitShort: "abc"}
	v3 := &version.Release{GitCommit: "def456", GitShort: "def"}

	if !v1.CompareCommit(v2) {
		t.Error("Identical commits should be equal")
	}

	if v1.CompareCommit(v3) {
		t.Error("Different commits should not be equal")
	}
}

func TestVersionValidate(t *testing.T) {
	t.Parallel()
	v := &version.Release{}

	// Test valid version
	validVersion := &version.Release{
		GitTag:    "v1.2.3",
		GitCommit: "abc123",
		GitShort:  "abc",
		BuildDate: "2024-01-01",
		GoVersion: "go1.21",
		Platform:  "linux/amd64",
	}

	if err := v.Validate(validVersion); err != nil {
		t.Errorf("Valid version should pass validation: %v", err)
	}

	// Test invalid versions
	testCases := []struct {
		name    string
		version *version.Release
	}{
		{"empty tag", &version.Release{GitTag: "", GitCommit: "abc", GitShort: "abc", BuildDate: "2024-01-01", GoVersion: "go1.21", Platform: "linux/amd64"}},
		{"empty commit", &version.Release{GitTag: "v1.2.3", GitCommit: "", GitShort: "abc", BuildDate: "2024-01-01", GoVersion: "go1.21", Platform: "linux/amd64"}},
		{"empty short", &version.Release{GitTag: "v1.2.3", GitCommit: "abc123", GitShort: "", BuildDate: "2024-01-01", GoVersion: "go1.21", Platform: "linux/amd64"}},
		{"empty build date", &version.Release{GitTag: "v1.2.3", GitCommit: "abc123", GitShort: "abc", BuildDate: "", GoVersion: "go1.21", Platform: "linux/amd64"}},
		{"empty go version", &version.Release{GitTag: "v1.2.3", GitCommit: "abc123", GitShort: "abc", BuildDate: "2024-01-01", GoVersion: "", Platform: "linux/amd64"}},
		{"empty platform", &version.Release{GitTag: "v1.2.3", GitCommit: "abc123", GitShort: "abc", BuildDate: "2024-01-01", GoVersion: "go1.21", Platform: ""}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := v.Validate(tc.version); err == nil {
				t.Errorf("Expected validation to fail for %s", tc.name)
			}
		})
	}
}
