package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"ticket_automation/internal/db"
	"ticket_automation/internal/worker"
)

func main() {
	ctx := context.Background()

	if err := loadEnvironment(); err != nil {
		log.Fatal(err)
	}

	if err := db.InitLocation(); err != nil {
		log.Fatal(err)
	}

	pool, err := connectDB(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	db.DB = pool

	worker.RunInParallel(ctx)
}

func loadEnvironment() error {
	_, currentFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(currentFile)

	envPath := filepath.Join(baseDir, ".env")

	if runtime.GOOS != "windows" {
		envPath = filepath.Join(baseDir, "..", "ENV", ".env")
	}

	log.Printf("Loading environment variables from: %s", envPath)

	if err := godotenv.Overload(envPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("Warning: .env file not found at %s", envPath)
		} else {
			return err
		}
	}

	return nil
}

func connectDB(ctx context.Context) (*pgxpool.Pool, error) {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	database := os.Getenv("DB_NAME")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASS")
	sslMode := os.Getenv("DB_SSLMODE")

	if port == "" {
		port = "5432"
	}

	if sslMode == "" {
		sslMode = "prefer"
	}

	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		user,
		password,
		host,
		port,
		database,
		sslMode,
	)

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	if maxConns := os.Getenv("DB_MAX_CONNS"); maxConns != "" {
		if value, err := strconv.ParseUint(maxConns, 10, 32); err == nil {
			config.MaxConns = int32(value)
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}