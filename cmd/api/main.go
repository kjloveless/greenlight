package main

import (
  "context"
  "database/sql"
  "flag"
  "fmt"
  "log/slog"
  "net/http"
  "os"
  "time"

  // Import the pq driver so that it can register itself with the database/sql
  // package. Note that we alias this import to the blank identifier, to stop
  // the Go compilter complaining that the package isn't being used.
  _ "github.com/lib/pq"
)

// Declare a string containing the application version number. Later in the
// book we'll generate this automatically at build time, but for now we'll just
// store the version number as a hard-coded global constant.
const version = "1.0.0"

// Define a config struct to hold all the configuration settings for our
// application. For now, the only configuration settings will be the network
// port that we want the server to listen on, and the name of the current
// operating environment for the application (development, staging, production,
// etc.). We will read in these configuration settings from command-line flags
// when the application starts.
type config struct {
  port  int
  env   string
  db    struct {
    dsn           string
    maxOpenConns  int
    maxIdleConns  int
    maxIdleTime   time.Duration
  }
}

// Define an application struct to hold the dependencies for our HTTP handlers,
// helpers, and middleware. At the moment this only contains a copy of the
// config struct and a logger, but it will grow to include a lot more as our
// build progresses.
type application struct {
  config  config
  logger  *slog.Logger
}

func main() {
  // Declare an instance of the config struct.
  var cfg config

  // Read the value of the port and env command-line flags into the config
  // struct. We default to using the port number 4000 and the environment
  // "development" if no corresponding flags are provided.
  flag.IntVar(&cfg.port, "port", 4000, "API server port")
  flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")
  // Read the DSN value from the db-dsn command-line flag into the config
  // struct. We default to using our development DSN if no flag is provided.
  flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("GREENLIGHT_DB_DSN"), "PostgreSQL DSN")
  // Read the connection pool settings from command-line flags into the config
  // struct. Notice that the default values we're using are the ones we
  // discussed above?
  flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 25, "PostgreSQL max open connections")
  flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 25, "PostgreSQL max idle connections")
  flag.DurationVar(&cfg.db.macIdleTime, "db-max-idle-time", 15*time.Minute,
    "PostgreSQL max connection idle time")
  flag.Parse()

  // Initialize a new structured logger which writes log entries to the
  // standard out stream.
  logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

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

  // Declare an instance of the application struct, containing the config
  // struct and the logger.
  app := &application{
    config: cfg,
    logger: logger,
  }

  // Declare a HTTP server which listens on the port provided in the config
  // struct, uses the servemux we created about as the handler, has some
  // sensible timeout settings and writes any log messages to the structured
  // logger at Error level.
  srv := &http.Server{
    Addr:         fmt.Sprintf(":%d", cfg.port),
    Handler:      app.routes(),
    IdleTimeout:  time.Minute,
    ReadTimeout:  5 * time.Second,
    WriteTimeout: 10 * time.Second,
    ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
  }

  // Start the HTTP server.
  logger.Info("starting server", "addr", srv.Addr, "env", cfg.env)

  err = srv.ListenAndServe()
  logger.Error(err.Error())
  os.Exit(1)
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
  db.SetConnMaxIdleTimeout(cfg.db.maxIdleTime)


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
