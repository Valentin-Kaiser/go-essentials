// Package version provides utilities to manage, retrieve, and validate version
// information of a Go application at runtime.
//
// This package captures key metadata about the application build, including the
// Git tag, full and short commit hashes, build date, Go runtime version, target
// platform, and the list of Go modules used in the build. These values are
// intended to be set at build time via linker flags (-ldflags), allowing
// embedding of dynamic version information within the compiled binary.
//
// The package also includes functions to parse semantic version components
// (major, minor, patch) from Git tags following the "vX.Y.Z" format, validate
// version structs, and compare version information between two versions.
//
// The module list is automatically populated at runtime using debug.BuildInfo
// (available since Go 1.12+), which extracts module dependencies embedded by the
// Go build system.
//
// Typical usage involves setting the version variables during build with
// `-ldflags`, for example in a Makefile:
//
//	GIT_TAG := $(shell git describe --tags)
//	GIT_COMMIT := $(shell git rev-parse HEAD)
//	GIT_SHORT := $(shell git rev-parse --short HEAD)
//	BUILD_TIME := $(shell date +%FT%T%z)
//	VERSION_PACKAGE := github.com/Valentin-Kaiser/go-core/version
//
//	go build -ldflags "-X $(VERSION_PACKAGE).GitTag=$(GIT_TAG) \
//	                 -X $(VERSION_PACKAGE).GitCommit=$(GIT_COMMIT) \
//	                 -X $(VERSION_PACKAGE).GitShort=$(GIT_SHORT) \
//	                 -X $(VERSION_PACKAGE).BuildDate=$(BUILD_TIME)" main.go
//
// The package defines the Version struct encapsulating all relevant fields,
// as well as the Module struct to represent individual Go module dependencies.
//
// Example:
//
//	v := version.GetVersion()
//	fmt.Printf("App version: %s (commit %s)\n", v.GitTag, v.GitShort)
package version

import (
	"errors"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

var (
	GitTag    = "v0.0.0"
	GitCommit = "unknown"
	GitShort  = "unknown"
	BuildDate = "unknown"
	GoVersion = runtime.Version()
	Platform  = runtime.GOOS + "/" + runtime.GOARCH
	Modules   = make([]*Module, 0)
)

var regex = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)`)

func init() {
	if info, available := debug.ReadBuildInfo(); available {
		for _, mod := range info.Deps {
			Modules = append(Modules, (&Module{}).fromBuildInfo(mod))
		}
	}
}

// Version represents the version information of the application.
// It includes the Git tag, commit hash, build date, Go version, platform, and a list of modules.
type Version struct {
	ID        uint64    `json:"-" gorm:"primaryKey"`
	GitTag    string    `json:"gitTag" gorm:"uniqueIndex:idx_version_module"`
	GitCommit string    `json:"gitCommit" gorm:"uniqueIndex:idx_version_module"`
	GitShort  string    `json:"gitShort"`
	BuildDate string    `json:"buildDate" gorm:"uniqueIndex:idx_version_module"`
	GoVersion string    `json:"goVersion" gorm:"uniqueIndex:idx_version_module"`
	Platform  string    `json:"platform" gorm:"uniqueIndex:idx_version_module"`
	Modules   []*Module `json:"modules" gorm:"-"`
}

// Module represents a module dependency of the application.
// It includes the module path, version, checksum, and an optional replacement module.
type Module struct {
	Path    string
	Version string
	Sum     string
	Replace *Module `json:",omitempty"`
}

// GetVersion returns the current version information of the application.
func GetVersion() *Version {
	return &Version{
		GitTag:    GitTag,
		GitCommit: GitCommit,
		GitShort:  GitShort,
		BuildDate: BuildDate,
		GoVersion: GoVersion,
		Platform:  Platform,
		Modules:   Modules,
	}
}

// Major returns the major version number from the Git tag.
func Major() int {
	return ParseTagSegment(GitTag, 0)
}

// Minor returns the minor version number from the Git tag.
func Minor() int {
	return ParseTagSegment(GitTag, 1)
}

// Patch returns the patch version number from the Git tag.
func Patch() int {
	return ParseTagSegment(GitTag, 2)
}

// String returns the version tag as a string without the "v" prefix.
func String() string {
	version := strings.SplitN(GitTag, "-", 2)[0]
	return strings.TrimPrefix(version, "v")
}

// IsGitTag checks if the provided tag is a valid Git tag in the format "vX.Y.Z".
func IsGitTag(tag string) bool {
	return regex.MatchString(tag)
}

// ParseTagSegment parses the specified segment (major, minor, or patch) from the Git tag.
func ParseTagSegment(tag string, n int) int {
	if !IsGitTag(tag) {
		return 0
	}

	version := strings.TrimPrefix(strings.SplitN(tag, "-", 2)[0], "v")
	segments := strings.Split(version, ".")
	if n >= len(segments) {
		log.Debug().Err(errors.New("index out of range")).Msgf("error parsing version segment at index %d", n)
		return 0
	}

	v, err := strconv.Atoi(segments[n])
	if err != nil {
		log.Debug().Err(err).Msgf("error parsing version segment at index %d", n)
		return 0
	}

	return v
}

// ParseTagVersion parses the version number from the Git tag.
func ParseTagVersion(tag string) string {
	if !IsGitTag(tag) {
		return ""
	}

	return strings.TrimPrefix(strings.SplitN(tag, "-", 2)[0], "v")
}

// Compare compares the Git tag and commit hash of the current version with another version.
func (v *Version) Compare(c *Version) bool {
	return v.CompareTag(c) && v.CompareCommit(c)
}

// CompareTag compares the Git tag of the current version with another version.
func (v *Version) CompareTag(c *Version) bool {
	return v.GitTag == c.GitTag
}

// CompareCommit compares the Git commit hash and short hash of the current version with another version.
func (v *Version) CompareCommit(c *Version) bool {
	return v.GitCommit == c.GitCommit && v.GitShort == c.GitShort
}

// Validate checks if the provided version information is valid.
func (v *Version) Validate(change *Version) error {
	if strings.TrimSpace(change.GitTag) == "" {
		return errors.New("tag cannot be empty")
	}

	if strings.TrimSpace(change.GitCommit) == "" {
		return errors.New("commit cannot be empty")
	}

	if strings.TrimSpace(change.GitShort) == "" {
		return errors.New("short commit cannot be empty")
	}

	if strings.TrimSpace(change.BuildDate) == "" {
		return errors.New("build date cannot be empty")
	}

	if strings.TrimSpace(change.GoVersion) == "" {
		return errors.New("go version cannot be empty")
	}

	if strings.TrimSpace(change.Platform) == "" {
		return errors.New("platform cannot be empty")
	}

	return nil
}

// fromBuildInfo populates the Module struct with information from the debug.Module.
func (m *Module) fromBuildInfo(mod *debug.Module) *Module {
	m.Path = mod.Path
	m.Version = mod.Version
	m.Sum = mod.Sum
	if mod.Replace != nil {
		m.Replace = &Module{}
		m.Replace.fromBuildInfo(mod.Replace)
	}
	return m
}
