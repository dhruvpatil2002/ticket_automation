package main

import (
	"context"
	"log"
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

	log.Println("Starting ticket automation service...")

	// 1. Load environment configuration
	if err := loadEnvironment(); err != nil {
		log.Fatalf("Failed to load environment: %v", err)
	}

	// 2. Initialize application timezone (IST / Asia/Kolkata)
	if err := db.InitLocation(); err != nil {
		log.Fatalf("Failed to initialize location: %v", err)
	}

	// 3. Initialize database connection pool
	if _, err := db.InitDB(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		log.Println("Closing database connections...")
		db.Close()
	}()

	// 4. Run worker tasks in parallel
	startTime := time.Now()
	log.Println("Executing workers in parallel...")
	worker.RunInParallel(ctx)
	log.Printf("All workers finished in %s", time.Since(startTime).Round(time.Millisecond))
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
				log.Printf("Loaded environment variables from: %s", path)
				return nil
			}
		}
	}

	log.Printf("Notice: No .env file found in candidates %v (using system environment)", candidatePaths)
	return nil
}
