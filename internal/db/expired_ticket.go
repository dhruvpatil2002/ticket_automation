package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func ExpiredTickets(ctx context.Context) {
	const function = "expired_tickets"

	tx, err := DB.Begin(ctx)
	if err != nil {
		fmt.Printf("%s unable to begin transaction: %v", logPrefix(function), err)
		return
	}
	defer tx.Rollback(ctx)

	var yottaMessage *string
	var yottaClosedCount *int64

	err = tx.QueryRow(ctx, `
		SELECT summary_message, closed_tickets_count
		FROM expire_panel_tickets_summary()
	`).Scan(&yottaMessage, &yottaClosedCount)

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		fmt.Printf("%s Yotta function failed: %v\n", logPrefix(function), err)
		return
	}

	fmt.Printf(
		"%s Expired Yotta Ticket - message=%v count=%v\n",
		logPrefix(function),
		yottaMessage,
		yottaClosedCount,
	)

	var safeboxMessage *string
	var safeboxClosedCount *int64

	err = tx.QueryRow(ctx, `
		SELECT summary_message, closed_tickets_count
		FROM public.expire_safebox_tickets_summary()
	`).Scan(&safeboxMessage, &safeboxClosedCount)

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		fmt.Printf("%s Safebox function failed: %v\n", logPrefix(function), err)
		return
	}

	fmt.Printf(
		"%s Expired Safebox Ticket - message=%v count=%v\n",
		logPrefix(function),
		safeboxMessage,
		safeboxClosedCount,
	)

	if err := tx.Commit(ctx); err != nil {
		fmt.Printf("%s commit failed: %v", logPrefix(function), err)
		return
	}

	fmt.Printf("%s Expired tickets processed successfully.\n", logPrefix(function))
}
