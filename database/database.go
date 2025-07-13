// Package database provides a robust and flexible abstraction over GORM,
// supporting both SQLite and MySQL/MariaDB as backend databases.
//
// It offers features such as automatic connection handling, schema migrations,
// and version tracking. The package also allows registering custom on-connect
// handlers that are executed once the database connection is successfully established.
//
// Core features:
//
//   - Automatic (re)connection with health checks and retry mechanism
//   - Support for SQLite and MySQL/MariaDB with configurable parameters
//   - Schema management and automatic migrations
//   - Versioning support using the go-core/version package
//   - Connection lifecycle management (Connect, Disconnect, AwaitConnection)
//   - Thread-safe access with `Execute(func(db *gorm.DB) error)` wrapper
//   - Registering on-connect hooks to perform actions like seeding or setup
//
// Example:
//
//	package main
//
//	import (
//		"fmt"
//		"time"
//
//		"github.com/Valentin-Kaiser/go-core/database"
//		"github.com/Valentin-Kaiser/go-core/flag"
//		"github.com/Valentin-Kaiser/go-core/version"
//		"gorm.io/gorm"
//	)
//
//	type User struct {
//		gorm.Model
//		Name     string
//		Email    string `gorm:"unique"`
//		Password string
//	}
//
//	func main() {
//		flag.Init()
//
//		database.RegisterSchema(&User{})
//		database.RegisterMigrationStep(version.Version{
//			GitTag:    "v1.0.0",
//			GitCommit: "abc123",
//		}, func(db *gorm.DB) error {
//			// Custom migration logic
//			return nil
//		})
//
//		database.Connect(time.Second, database.Config{
//			Driver: "sqlite",
//			Name:   "test",
//		})
//		defer database.Disconnect()
//
//		database.AwaitConnection()
//
//		var ver version.Version
//		err := database.Execute(func(db *gorm.DB) error {
//			return db.First(&ver).Error
//		})
//		if err != nil {
//			panic(err)
//		}
//
//		fmt.Println("Version:", ver.GitTag, ver.GitCommit)
//	}
package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/Valentin-Kaiser/go-core/flag"
	"github.com/Valentin-Kaiser/go-core/interruption"
	"github.com/Valentin-Kaiser/go-core/version"
	"github.com/Valentin-Kaiser/go-core/zlog"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db               *gorm.DB
	connected        atomic.Bool
	failed           atomic.Bool
	cancel           atomic.Bool
	done             = make(chan bool)
	onConnectHandler []func(db *gorm.DB, config Config) error
)

// Execute executes a function with a database connection.
// It will return an error if the database is not connected or if the function returns an error.
// The function will be executed with a new session, so it will not affect the current transaction.
func Execute(call func(db *gorm.DB) error) error {
	if !connected.Load() || db == nil {
		return apperror.NewErrorf("database is not connected")
	}

	err := call(db.Session(&gorm.Session{}))
	if err != nil {
		return err
	}

	return nil
}

// Connected returns true if the database is connected, false otherwise
func Connected() bool {
	return connected.Load()
}

// Reconnect will set the connected state to false and trigger a reconnect
func Reconnect() {
	log.Trace().Msg("[Database] reconnecting...")
	connected.Store(false)
	failed.Store(false)
}

// Disconnect will stop the database connection and wait for the connection to be closed
func Disconnect() {
	log.Trace().Msg("[Database] closing connection...")
	cancel.Store(true)
	<-done
	log.Trace().Msg("[Database] connection closed")
}

// AwaitConnection will block until the database is connected
func AwaitConnection() {
	for !connected.Load() {
		time.Sleep(time.Second)
	}
}

// Connect will try to connect to the database every interval until it is connected
// It will also check if the connection is still alive every interval and reconnect if it is not
func Connect(interval time.Duration, config Config) {
	go func() {
		for {
			func() {
				defer interruption.Handle()
				defer time.Sleep(interval)

				// If we are not connected to the database, try to connect
				if !connected.Load() {
					var err error
					db, err = connect(config)
					if err != nil {
						// Prevent spamming the logs with connection errors
						if !failed.Load() {
							log.Error().Err(err).Msg("[Database] connection failed")
						}
						failed.Store(true)
						return
					}

					onConnect(config)
					for _, handler := range onConnectHandler {
						err := handler(db, config)
						if err != nil {
							log.Error().Err(err).Msg("[Database] onConnect handler failed")
							failed.Store(true)
							return
						}
					}

					if failed.Load() {
						log.Info().Msg("[Database] connection restored")
					}
					failed.Store(false)
					connected.Store(true)
					log.Debug().Msg("[Database] connection established")
					return
				}

				// Verify that we are indeed connected, if 'SELECT 1;' fails we assume
				// that the database is currently unavailable
				err := db.Exec("SELECT 1;").Error
				if err != nil && connected.Load() {
					log.Error().Err(err).Msg("[Database] connection lost")
					connected.Store(false)
					failed.Store(true)
				}
			}()

			if cancel.Load() {
				done <- true
				return
			}
		}
	}()
}

// RegisterOnConnectHandler registers a function that will be called when the database connection is established
func RegisterOnConnectHandler(handler func(db *gorm.DB, config Config) error) {
	if handler == nil {
		return
	}

	onConnectHandler = append(onConnectHandler, handler)
}

// connect will try to connect to the database and return the connection
func connect(config Config) (*gorm.DB, error) {
	// Silence gorm internal logging
	newLogger := logger.New(
		&log.Logger,
		logger.Config{
			SlowThreshold: time.Second,
			LogLevel:      logger.Silent,
		},
	)
	// If we are in trace loglevel, enable gorm logging
	if zlog.Logger().GetLevel() < zerolog.TraceLevel && flag.Debug {
		newLogger = logger.New(
			&log.Logger,
			logger.Config{
				SlowThreshold: time.Second,
				LogLevel:      logger.Info,
			},
		)
	}
	cfg := &gorm.Config{Logger: newLogger}

	switch config.Driver {
	case "sqlite":
		var dbPath string
		if config.Name == ":memory:" {
			// Use in-memory SQLite database
			dbPath = ":memory:?cache=shared"
		} else {
			// Use file-based SQLite database
			if _, err := os.Stat(flag.Path); os.IsNotExist(err) {
				err := os.Mkdir(flag.Path, 0750)
				if err != nil {
					return nil, err
				}
			}
			dbPath = filepath.Join(flag.Path, config.Name+".db?cache=shared")
		}

		var err error
		db, err = gorm.Open(sqlite.Open(dbPath), cfg)
		if err != nil {
			return nil, err
		}

		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}

		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)

		return db, nil
	case "mysql", "mariadb":
		dsn := fmt.Sprintf(
			"%v:%v@tcp(%v:%v)/%v?charset=utf8mb4&parseTime=True&loc=Local",
			config.User,
			config.Password,
			config.Host,
			config.Port,
			config.Name,
		)

		var err error
		db, err = gorm.Open(mysql.Open(dsn), cfg)
		if err != nil {
			return nil, err
		}

		return db, nil
	default:
		return nil, apperror.NewErrorf("unsupported database driver: %v", config.Driver)
	}
}

// onConnect is an internal Event handler that will be called when a Database Connection is successfully established
// It will run the migration setup and check if the database is up to date
func onConnect(config Config) {
	err := setup(db)
	if err != nil {
		log.Fatal().Err(err).Msgf("[Database] schema setup failed")
	}

	// Check for the current version in the database
	revision := version.GetVersion()
	err = db.Where("git_tag = ? AND git_commit = ?", revision.GitTag, revision.GitCommit).First(&version.Version{}).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Fatal().Err(err).Msgf("[Database] version check failed")
	}

	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = migrateSchema(db)
		if err != nil {
			log.Fatal().Err(err).Msgf("[Database] schema migration failed")
		}

		for _, steps := range getMigrationSteps() {
			if len(steps) == 0 {
				continue
			}

			// Check if the key exists as a revision
			err = db.Where("git_tag = ? AND git_commit = ?", steps[0].Version.GitTag, steps[0].Version.GitCommit).First(&version.Version{}).Error
			if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
				for _, step := range steps {
					// Execute all migration step actions for this version
					err = step.Action(db)
					if err != nil {
						log.Fatal().Err(err).Msgf("[Database] migration failed")
					}
				}

				// Create the version record for this migration step
				err = db.Create(&steps[0].Version).Error
				if err != nil {
					log.Fatal().Err(err).Msgf("[Database] version creation failed")
				}
			}
		}

		err = db.Create(&revision).Error
		if err != nil {
			log.Fatal().Err(err).Msgf("[Database] version creation failed")
		}
	}

	if config.Driver == "sqlite" {
		err = db.Exec("VACUUM;").Error
		if err != nil {
			log.Warn().Err(err).Msgf("[Database] vacuum failed")
		}
	}
}
