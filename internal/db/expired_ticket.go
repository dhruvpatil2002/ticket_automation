package db

import (
	"context"
	"errors"
	"log"

	"github.com/jackc/pgx/v5"
)

func ExpiredTickets(ctx context.Context) {
	const function = "expired_tickets"

	tx, err := DB.Begin(ctx)
	if err != nil {
		log.Printf("%s unable to begin transaction: %v", logPrefix(function), err)
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
		log.Printf("%s Yotta function failed: %v", logPrefix(function), err)
		return
	}

	log.Printf(
		"%s Expired Yotta Ticket - message=%v count=%v",
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
		log.Printf("%s Safebox function failed: %v", logPrefix(function), err)
		return
	}

	log.Printf(
		"%s Expired Safebox Ticket - message=%v count=%v",
		logPrefix(function),
		safeboxMessage,
		safeboxClosedCount,
	)

	if err := tx.Commit(ctx); err != nil {
		log.Printf("%s commit failed: %v", logPrefix(function), err)
		return
	}

	log.Printf("%s Expired tickets processed successfully.", logPrefix(function))
}