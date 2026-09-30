package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func ExpiredTickets(ctx context.Context) error {
	const function = "expired_tickets"

	// 1. Enforce a hard timeout guard to avoid hanging long transactions
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 2. Open transaction
	tx, err := DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%s unable to begin transaction: %w", logPrefix(function), err)
	}
	defer tx.Rollback(ctx)

	var (
		yottaMsg, safeboxMsg     *string
		yottaCount, safeboxCount *int64
	)

	const combinedQuery = `
		SELECT 
			y.summary_message, y.closed_tickets_count,
			s.summary_message, s.closed_tickets_count
		FROM expire_panel_tickets_summary() y
		CROSS JOIN public.expire_safebox_tickets_summary() s;
	`

	err = tx.QueryRow(ctx, combinedQuery).Scan(
		&yottaMsg, &yottaCount,
		&safeboxMsg, &safeboxCount,
	)

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s expiration stored procedures failed: %w", logPrefix(function), err)
	}

	// 4. Safe formatting to avoid printing pointer addresses (0x...)
	logResult(function, "Yotta", yottaMsg, yottaCount)
	logResult(function, "Safebox", safeboxMsg, safeboxCount)

	// 5. Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s commit failed: %w", logPrefix(function), err)
	}

	fmt.Printf("%s Expired tickets processed successfully.\n", logPrefix(function))
	return nil
}

// logResult formats messages cleanly even if stored procedures return SQL NULLs
func logResult(function, label string, msg *string, count *int64) {
	messageVal := ""
	var countVal int64

	if msg != nil {
		messageVal = *msg
	}
	if count != nil {
		countVal = *count
	}

	fmt.Printf(
		"%s Expired %s Ticket - message=%s count=%d\n",
		logPrefix(function),
		label,
		messageVal,
		countVal,
	)
}
