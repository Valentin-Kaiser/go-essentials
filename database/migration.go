package database

import (
	"sync"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/Valentin-Kaiser/go-core/version"
	"gorm.io/gorm"
)

// Migrations is a list of SQL statements to be executed as part of the migration process
// The key represents the version of the database schema change
// If the version does not exist in the database, the SQL statement will be executed
var (
	mutex          = &sync.RWMutex{}
	migrationSteps = map[string]map[string][]Step{}
	schema         = make([]interface{}, 0)
)

// Step represents a migration step with a version and an action to be performed
// The action is a function that takes a gorm.DB instance and returns an error
type Step struct {
	Version version.Release
	Action  func(db *gorm.DB) error
}

// RegisterSchema registers a new schema model to be migrated
func RegisterSchema(models ...interface{}) {
	schema = append(schema, models...)
}

// RegisterMigrationStep registers a new migration step with a version and an action
// The action is a function that takes a gorm.DB instance and returns an error
// The version is a struct that must contain the Git tag and commit hash of the migration step
func RegisterMigrationStep(version version.Release, action func(db *gorm.DB) error) {
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := migrationSteps[version.GitTag]; !ok {
		migrationSteps[version.GitTag] = make(map[string][]Step)
	}

	if _, ok := migrationSteps[version.GitTag][version.GitCommit]; !ok {
		migrationSteps[version.GitTag][version.GitCommit] = make([]Step, 0)
	}

	if version.GitTag == "" || version.GitCommit == "" {
		panic(apperror.NewErrorf("version information is not set"))
	}

	migrationSteps[version.GitTag][version.GitCommit] = append(migrationSteps[version.GitTag][version.GitCommit], Step{
		Version: version,
		Action:  action,
	})
}

// setup initializes the database schema with the version table
func setup(db *gorm.DB) error {
	err := db.AutoMigrate(
		&version.Release{},
	)
	if err != nil {
		return apperror.NewError("failed to migrate database schema").AddError(err)
	}

	return nil
}

// migrateSchema automatically migrates the database with the database model structs
func migrateSchema(db *gorm.DB) error {
	if len(schema) == 0 {
		return nil
	}
	err := db.AutoMigrate(schema...)
	if err != nil {
		return apperror.NewError("failed to migrate database schema").AddError(err)
	}

	return nil
}

// getMigrationSteps returns all registered migration steps
func getMigrationSteps() [][]Step {
	mutex.RLock()
	defer mutex.RUnlock()

	steps := make([][]Step, 0)
	for _, step := range migrationSteps {
		for _, s := range step {
			steps = append(steps, s)
		}
	}

	return steps
}
