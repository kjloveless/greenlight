package main

import (
	"context"
	"database/sql"
	"expvar"
	"flag"
  "fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kjloveless/greenlight/internal/data"
	"github.com/kjloveless/greenlight/internal/mailer"
  "github.com/kjloveless/greenlight/internal/vcs"

	"github.com/joho/godotenv"

	// Import the pq driver so that it can register itself with the database/sql
	// package. Note that we alias this import to the blank identifier, to stop
	// the Go compilter complaining that the package isn't being used.
	_ "github.com/lib/pq"
)

// Make version a variable (rather than a constant) and set its value to
// vcs.Version().
var (
  version = vcs.Version()
)

// Define a config struct to hold all the configuration settings for our
// application. For now, the only configuration settings will be the network
// port that we want the server to listen on, and the name of the current
// operating environment for the application (development, staging, production,
// etc.). We will read in these configuration settings from command-line flags
// when the application starts.
type config struct {
	port int
	env  string
	db   struct {
		dsn          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  time.Duration
	}
	// Add a new limiter struct containing fields for the requests-per-second and
	// burst values, and a boolean field which we can use to enable/disable rate
	// limiting altogether.
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
	// Add a cors struct and trustedOrigins field with the type []string.
	cors struct {
		trustedOrigins []string
	}
}

// Define an application struct to hold the dependencies for our HTTP handlers,
// helpers, and middleware. At the moment this only contains a copy of the
// config struct and a logger, but it will grow to include a lot more as our
// build progresses.
// Include a sync.WaitGroup in the application struct. The zero-value for a
// sync.WaitGroup type is a valid useable, sync.WaitGroup with a 'counter'
// value of 0, so we don't need to do anything else to initialize it before we
// can use it.
type application struct {
	config config
	logger *slog.Logger
	models data.Models
	mailer *mailer.Mailer
	wg     sync.WaitGroup
}

func main() {
	// Initialize a new structured logger which writes log entries to the
	// standard out stream.
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Load .env file to read in Mailtrap credentials
	err := godotenv.Load(".env")
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	smtp_username := os.Getenv("SMTP_USERNAME")
	smtp_password := os.Getenv("SMTP_PASSWORD")

	// Declare an instance of the config struct.
	var cfg config

	// Read the value of the port and env command-line flags into the config
	// struct. We default to using the port number 4000 and the environment
	// "development" if no corresponding flags are provided.
	flag.IntVar(&cfg.port, "port", 4000, "API server port")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")
	// Use the empty string "" as the default value for the db-dsn command-line
	// flag, rather than os.Getenv("GREENLIGHT_DB_DSN") like we were previously.
	flag.StringVar(&cfg.db.dsn, "db-dsn", "", "PostgreSQL DSN")
	// Read the connection pool settings from command-line flags into the config
	// struct. Notice that the default values we're using are the ones we
	// discussed above?
	flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL max open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 25, "PostgreSQL max idle connections")
	flag.DurationVar(&cfg.db.maxIdleTime, "db-max-idle-time", 15*time.Minute,
		"PostgreSQL max connection idle time")
	// Create command line flags to read the setting values into the config
	// struct. Notice that we use true as the default for the 'enabled' setting?
	flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 2,
		"Rate limiter maximum requests per second")
	flag.IntVar(&cfg.limiter.burst, "limiter-burst", 4, "Rate limiter maximum burst")
	flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "enable rate limiter")

	// Read the SMTP server configuration settings into the config struct, using
	// the Mailtrap settings as the default values. IMPORTANT: If you're
	// following along, make sure to replace the default values for smtp-username
	// and smtp-password with your own Mailtrap credentials.
	flag.StringVar(&cfg.smtp.host, "smtp-host", "sandbox.smtp.mailtrap.io", "SMTP host")
	flag.IntVar(&cfg.smtp.port, "smtp-port", 25, "SMTP port")
	flag.StringVar(&cfg.smtp.username, "smtp-username", smtp_username, "SMTP username")
	flag.StringVar(&cfg.smtp.password, "smtp-password", smtp_password, "SMTP password")
	flag.StringVar(&cfg.smtp.sender, "smtp-sender",
		"Greenlight <no-reply@greenlight.loveless.dev>", "SMTP sender")

	// Use the flag.Func() function to process the -cors-trusted-origins command
	// line flag. In this we use the strings.Fields() function to split the flag
	// value into a slice based on whitespace characters and assign it to our
	// config struct. Importantly, if the -cors-trusted-origins flag is not
	// present, contains the empty string, or contains only whitespace, then
	// strings.Fields() will return an empty []string slice.
	flag.Func("cors-trusted-origins", "Trusted CORS origins (space separated)",
		func(val string) error {
			cfg.cors.trustedOrigins = strings.Fields(val)
			return nil
		})

  // Create a new version boolean flag with the default value of false.
  displayVersion := flag.Bool("version", false, "Display version and exit.")

	flag.Parse()

  // If the version flag is true, then print out the version number and
  // immediately exit.
  if *displayVersion {
    fmt.Printf("Version:\t%s\n", version)
    os.Exit(0)
  }

	// Call the openDB() helper function (see below) to create the connection
	// pool, passing in the config struct. If this returns an error, we log it
	// and exit the application immediately.
	db, err := openDB(cfg)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	// Defer a call to db.Close() so that the connection pool is closed before
	// the main() function exits.
	defer db.Close()

	// Also log a messge to say that the connection pool has been successfully
	// established.
	logger.Info("database connection pool established")

	// Initialize a new Mailer instance using the settings from the command line
	// flags.
	mailer, err := mailer.New(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username,
		cfg.smtp.password, cfg.smtp.sender)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	// Publish a new "version" variable in the expvar handler containing our
	// application version number (currently the constant "1.0.0").
	expvar.NewString("version").Set(version)

	// Publish the number of active goroutines.
	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))

	// Publish the database connection pool statistics.
	expvar.Publish("database", expvar.Func(func() any {
		return db.Stats()
	}))

	// Publish the current Unix timestamp.
	expvar.Publish("timestamp", expvar.Func(func() any {
		return time.Now().Unix()
	}))

	// Declare an instance of the application struct, containing the config
	// struct and the logger.
	app := &application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db),
		mailer: mailer,
	}

	err = app.serve()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

// The openDB() function returns a sql.DB connection pool.
func openDB(cfg config) (*sql.DB, error) {
	// Use sql.Open() to create an empty connection pool, using the DSN from the
	// config struct.
	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	// Set the maximum number of open (in-use + idle) connections in the pool.
	// Note that passing a value less than or equal to 0 will mean there is no
	// limit.
	db.SetMaxOpenConns(cfg.db.maxOpenConns)

	// Set the maximum number of idle connections in the pool. Again, passing a
	// value less than or equal to 0 will mean there is no limit.
	db.SetMaxIdleConns(cfg.db.maxIdleConns)

	// Set the maximum idle timeout for connections in the pool. Passing a
	// duration less than or equal to 0 will mean that connections are not closed
	// due to their idle time.
	db.SetConnMaxIdleTime(cfg.db.maxIdleTime)

	// Create a context with a 5-second timeout deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use PingContext() to establish a new connection to the database, passing
	// in the context we created above as a parameter. If the connection couldn't
	// be established successfully within the 5 second deadline, then this will
	// return an error. If we get this error, or any other, we close the
	// connection pool and return the error.
	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}

	// Return the sql.DB connection pool.
	return db, nil
}
