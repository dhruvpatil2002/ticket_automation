package db

import (
	"context"
	"fmt"
)

func GetClosedTransactions(ctx context.Context) {

	const function = "closed_transactions"

	tx, err := DB.Begin(ctx)
	if err != nil {
		fmt.Printf("%s unable to begin transaction: %v\n", logPrefix(function), err)
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT 
			'normal' AS source,
			tat.id AS transaction_id,
			pt.id AS parent_ticket_id
		FROM panel_ticketassignmenttransaction tat
		JOIN panel_ticket pt ON tat.parent_ticket_id = pt.id
		WHERE pt.closed = TRUE

		UNION ALL

		SELECT 
			'safebox' AS source,
			tat.id AS transaction_id,
			pt.id AS parent_ticket_id
		FROM panel_ticketassignmenttransaction_safebox tat
		JOIN panel_ticket pt ON tat.parent_ticket_id = pt.id
		WHERE pt.closed = TRUE

		UNION ALL

		SELECT 
			'safeboxai' AS source,
			tat.id AS transaction_id,
			pt.id AS parent_ticket_id
		FROM panel_ticketassignmenttransaction_safeboxai tat
		JOIN panel_ticket pt ON tat.parent_ticket_id = pt.id
		WHERE pt.closed = TRUE
	`)
	if err != nil {
		fmt.Printf("%s query failed: %v\n", logPrefix(function), err)
		return
	}
	defer rows.Close()

	var normalIDs []int64
	var safeboxIDs []int64
	var safeboxAIIDs []int64
	parentIDs := make(map[int64]struct{})

	for rows.Next() {
		var source string
		var transactionID, parentID int64

		if err := rows.Scan(&source, &transactionID, &parentID); err != nil {
			fmt.Printf("%s scan failed: %v,\n", logPrefix(function), err)
			return
		}

		parentIDs[parentID] = struct{}{}

		switch source {
		case "normal":
			normalIDs = append(normalIDs, transactionID)
		case "safebox":
			safeboxIDs = append(safeboxIDs, transactionID)
		case "safeboxai":
			safeboxAIIDs = append(safeboxAIIDs, transactionID)
		}
	}

	if err := rows.Err(); err != nil {
		fmt.Printf("%s rows failed: %v\n", logPrefix(function), err)
		return
	}

	if len(parentIDs) == 0 {
		fmt.Printf("%s No closed transactions to process.\n", logPrefix(function))
		return
	}

	parentIDList := make([]int64, 0, len(parentIDs))
	for id := range parentIDs {
		parentIDList = append(parentIDList, id)
	}

	_, err = tx.Exec(ctx, `
		UPDATE panel_ticket AS child
		SET
			closed = TRUE,
			closed_at = parent.closed_at,
			"isAccepted" = TRUE,
			accepted_at = parent.accepted_at,
			comments = parent.comments
		FROM panel_ticket_children_tickets ct
		JOIN panel_ticket AS parent ON ct.from_ticket_id = parent.id
		WHERE child.id = ct.to_ticket_id
		AND parent.id = ANY($1)
	`, parentIDList)

	if err != nil {
		fmt.Printf("%s child update failed: %v\n", logPrefix(function), err)
		return
	}

	deleteIDs := func(table string, ids []int64) error {
		if len(ids) == 0 {
			return nil
		}

		query := fmt.Sprintf(
			`DELETE FROM %s WHERE id = ANY($1)`,
			table,
		)

		_, err := tx.Exec(ctx, query, ids)
		return err
	}

	if err := deleteIDs("panel_ticketassignmenttransaction", normalIDs); err != nil {
		fmt.Printf("%s normal transaction delete failed: %v\n", logPrefix(function), err)
		return
	}

	if err := deleteIDs("panel_ticketassignmenttransaction_safebox", safeboxIDs); err != nil {
		fmt.Printf("%s safebox transaction delete failed: %v\n", logPrefix(function), err)
		return
	}

	if err := deleteIDs("panel_ticketassignmenttransaction_safeboxai", safeboxAIIDs); err != nil {
		fmt.Printf("%s safebox AI transaction delete failed: %v\n", logPrefix(function), err)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Printf("%s commit failed: %v\n", logPrefix(function), err)
		return
	}

	fmt.Printf(
		"%s Processed %d normal, %d safebox and %d safebox AI transactions.\n",
		logPrefix(function),
		len(normalIDs),
		len(safeboxIDs),
		len(safeboxAIIDs),
	)
}
