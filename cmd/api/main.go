package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"ticket_automation/internal/db"
	"ticket_automation/internal/worker"
)

func main() {
	// Create context that listens for termination signals (Ctrl+C, Docker stop, SIGTERM)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("Starting ticket automation service...")

	// 1. Load environment configuration
	if err := loadEnvironment(); err != nil {
		fmt.Printf("Failed to load environment: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize application timezone (IST / Asia/Kolkata)
	if err := db.InitLocation(); err != nil {
		fmt.Printf("Failed to initialize location: %v\n", err)
		os.Exit(1)
	}

	// 3. Initialize database connection pool
	if _, err := db.InitDB(ctx); err != nil {
		fmt.Printf("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		fmt.Println("Closing database connections...")
		db.Close()
	}()

	// 4. Run worker tasks in parallel
	startTime := time.Now()
	fmt.Println("Executing workers in parallel...")
	worker.RunInParallel(ctx)
	fmt.Printf("All workers finished in %s\n", time.Since(startTime).Round(time.Millisecond))
}

// loadEnvironment searches for .env in the working directory, root directory, and base directories.
func loadEnvironment() error {
	_, currentFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(currentFile)
	rootDir := filepath.Dir(filepath.Dir(baseDir))

	candidatePaths := []string{
		".env",
		filepath.Join(rootDir, ".env"),
		filepath.Join(baseDir, ".env"),
	}

	if runtime.GOOS != "windows" {
		candidatePaths = append([]string{filepath.Join(baseDir, "..", "ENV", ".env")}, candidatePaths...)
	}

	for _, path := range candidatePaths {
		if _, err := os.Stat(path); err == nil {
			if err := godotenv.Overload(path); err == nil {
				fmt.Printf("Loaded environment variables from: %s\n", path)
				return nil
			}
		}
	}

	fmt.Printf("Notice: No .env file found in candidates %v (using system environment)\n", candidatePaths)
	return nil
}
