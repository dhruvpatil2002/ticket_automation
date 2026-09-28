package db

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ISTLocation = "Asia/Kolkata"
	NoAgentID   = 602
)

type Ticket struct {
	ID     int64
	SiteID *int64
}

type Agent struct {
	ProfileID int64
	UserID    int64
	BUID      int64
}

type Assignment struct {
	TicketID  int64
	UserID    int64
	ProfileID int64
}

type UserKey struct {
	ProfileID int64
	UserID    int64
}

var (
	DB  *pgxpool.Pool
	loc *time.Location
)

// InitDB initializes the database connection pool using environment variables and sets it to DB.
func InitDB(ctx context.Context) (*pgxpool.Pool, error) {
	pool, err := ConnectDB(ctx)
	if err != nil {
		return nil, err
	}
	DB = pool
	return pool, nil
}

// Close closes the database connection pool if open.
func Close() {
	if DB != nil {
		DB.Close()
	}
}

// ConnectDB builds a connection pool based on environment variables with sensible defaults.
func ConnectDB(ctx context.Context) (*pgxpool.Pool, error) {
	host := getEnvOrDefault("DB_HOST", "localhost")
	port := getEnvOrDefault("DB_PORT", "5432")
	database := getEnvOrDefault("DB_NAME", "ticket_automation")
	user := getEnvOrDefault("DB_USER", "ticket_automation")
	password := os.Getenv("DB_PASS")
	sslMode := getEnvOrDefault("DB_SSLMODE", "disable")

	// Safely construct DSN handling special characters in password/user
	connURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   database,
	}
	query := connURL.Query()
	query.Set("sslmode", sslMode)
	connURL.RawQuery = query.Encode()

	config, err := pgxpool.ParseConfig(connURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Connection pool tuning
	if maxConnsStr := os.Getenv("DB_MAX_CONNS"); maxConnsStr != "" {
		if val, err := strconv.ParseUint(maxConnsStr, 10, 32); err == nil && val > 0 {
			config.MaxConns = int32(val)
		}
	} else {
		config.MaxConns = 20
	}

	config.MinConns = 2
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection with timeout
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed on %s:%s: %w", host, port, err)
	}

	return pool, nil
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// InitLocation initializes the timezone location for IST.
func InitLocation() error {
	var err error
	loc, err = time.LoadLocation(ISTLocation)
	return err
}

func now() time.Time {
	if loc == nil {
		return time.Now()
	}
	return time.Now().In(loc)
}

func logPrefix(function string) string {
	return fmt.Sprintf("%s || %s ||", now().Format(time.RFC3339), function)
}

