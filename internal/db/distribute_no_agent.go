package db

import (
	"context"
	"log"
	"time"
)

func DistributeNoAgent(ctx context.Context) {
	const function = "distribute_no_agent"

	tx, err := DB.Begin(ctx)
	if err != nil {
		log.Printf("%s unable to begin transaction: %v", logPrefix(function), err)
		return
	}
	defer tx.Rollback(ctx)

	currentTime := now()
	compareTime := currentTime.Add(-5 * time.Minute)

	rows, err := tx.Query(ctx, `
		SELECT pt.id, pt.site_id
		FROM panel_ticket pt
		JOIN panel_site ps ON ps.id = pt.site_id
		WHERE pt.assigned_to_id = $1
		  AND pt.is_parent = TRUE
		  AND pt.closed = FALSE
		  AND ps.enterprise_id != 34
	`, NoAgentID)
	if err != nil {
		log.Printf("%s ticket query failed: %v", logPrefix(function), err)
		return
	}
	defer rows.Close()

	var tickets []Ticket

	for rows.Next() {
		var ticket Ticket

		if err := rows.Scan(&ticket.ID, &ticket.SiteID); err != nil {
			log.Printf("%s ticket scan failed: %v", logPrefix(function), err)
			return
		}

		tickets = append(tickets, ticket)
	}

	if len(tickets) == 0 {
		log.Printf("%s No parent tickets to reassign.", logPrefix(function))
		return
	}

	siteIDs := make([]int64, 0)
	siteSet := make(map[int64]struct{})

	for _, ticket := range tickets {
		if ticket.SiteID != nil {
			if _, exists := siteSet[*ticket.SiteID]; !exists {
				siteIDs = append(siteIDs, *ticket.SiteID)
				siteSet[*ticket.SiteID] = struct{}{}
			}
		}
	}

	if len(siteIDs) == 0 {
		log.Printf("%s No valid site IDs found.", logPrefix(function))
		return
	}

	rows, err = tx.Query(ctx, `
		SELECT id, bu_id
		FROM panel_site
		WHERE id = ANY($1)
	`, siteIDs)
	if err != nil {
		log.Printf("%s site query failed: %v", logPrefix(function), err)
		return
	}

	siteBUMap := make(map[int64]int64)

	for rows.Next() {
		var siteID, buID int64

		if err := rows.Scan(&siteID, &buID); err != nil {
			rows.Close()
			log.Printf("%s site scan failed: %v", logPrefix(function), err)
			return
		}

		siteBUMap[siteID] = buID
	}
	rows.Close()

	buIDs := make([]int64, 0)
	buSet := make(map[int64]struct{})

	for _, buID := range siteBUMap {
		if _, exists := buSet[buID]; !exists {
			buIDs = append(buIDs, buID)
			buSet[buID] = struct{}{}
		}
	}

	if len(buIDs) == 0 {
		log.Printf("%s No BU IDs found.", logPrefix(function))
		return
	}

	rows, err = tx.Query(ctx, `
		SELECT up.id, up.user_id, bub.business_unit_id
		FROM panel_user_profile up
		JOIN auth_user u ON up.user_id = u.id
		JOIN panel_user_profile_bu bub
			ON bub.user_profile_id = up.id
		JOIN panel_business_unit pbu
			ON pbu.id = bub.business_unit_id
		WHERE up.on_break = FALSE
		  AND up.on_shift_end = FALSE
		  AND up.role = 'agent'
		  AND bub.business_unit_id = ANY($1)
		  AND up.last_active >= $2
		  AND pbu.enterprise_id != 34
	`, buIDs, compareTime)
	if err != nil {
		log.Printf("%s agent query failed: %v", logPrefix(function), err)
		return
	}
	defer rows.Close()

	userBUMap := make(map[UserKey]map[int64]struct{})

	for rows.Next() {
		var profileID, userID, buID int64

		if err := rows.Scan(&profileID, &userID, &buID); err != nil {
			log.Printf("%s agent scan failed: %v", logPrefix(function), err)
			return
		}

		key := UserKey{
			ProfileID: profileID,
			UserID:    userID,
		}

		if userBUMap[key] == nil {
			userBUMap[key] = make(map[int64]struct{})
		}

		userBUMap[key][buID] = struct{}{}
	}

	userQueue := make([]UserKey, 0, len(userBUMap))
	for user := range userBUMap {
		userQueue = append(userQueue, user)
	}

	if len(userQueue) == 0 {
		log.Printf("%s No available agents found.", logPrefix(function))
		return
	}

	buTicketMap := make(map[int64][]Ticket)

	for _, ticket := range tickets {
		if ticket.SiteID == nil {
			continue
		}

		if buID, exists := siteBUMap[*ticket.SiteID]; exists {
			buTicketMap[buID] = append(buTicketMap[buID], ticket)
		}
	}

	var assignments []Assignment
	parentToUser := make(map[int64]Assignment)
	ticketsPerProfile := make(map[int64][]int64)

	for buID, buTickets := range buTicketMap {
		var eligible []UserKey

		for _, user := range userQueue {
			if _, exists := userBUMap[user][buID]; exists {
				eligible = append(eligible, user)
			}
		}

		if len(eligible) == 0 {
			log.Printf(
				"%s BU:%d has no eligible agents for %d tickets.",
				logPrefix(function),
				buID,
				len(buTickets),
			)
			continue
		}

		for index, ticket := range buTickets {
			user := eligible[index%len(eligible)]

			assignment := Assignment{
				TicketID:  ticket.ID,
				UserID:    user.UserID,
				ProfileID: user.ProfileID,
			}

			assignments = append(assignments, assignment)
			parentToUser[ticket.ID] = assignment
			ticketsPerProfile[user.ProfileID] =
				append(ticketsPerProfile[user.ProfileID], ticket.ID)
		}
	}

	if len(assignments) == 0 {
		log.Printf("%s No assignments generated.", logPrefix(function))
		return
	}

	parentIDs := make([]int64, 0, len(parentToUser))
	for ticketID := range parentToUser {
		parentIDs = append(parentIDs, ticketID)
	}

	childRows, err := tx.Query(ctx, `
		SELECT from_ticket_id, to_ticket_id
		FROM panel_ticket_children_tickets
		WHERE from_ticket_id = ANY($1)
	`, parentIDs)
	if err != nil {
		log.Printf("%s child query failed: %v", logPrefix(function), err)
		return
	}

	for childRows.Next() {
		var fromID, toID int64

		if err := childRows.Scan(&fromID, &toID); err != nil {
			childRows.Close()
			log.Printf("%s child scan failed: %v", logPrefix(function), err)
			return
		}

		if parentAssignment, exists := parentToUser[fromID]; exists {
			assignments = append(assignments, Assignment{
				TicketID:  toID,
				UserID:    parentAssignment.UserID,
				ProfileID: parentAssignment.ProfileID,
			})

			ticketsPerProfile[parentAssignment.ProfileID] =
				append(ticketsPerProfile[parentAssignment.ProfileID], toID)
		}
	}
	childRows.Close()

	for _, assignment := range assignments {
		_, err = tx.Exec(ctx, `
			UPDATE panel_ticket
			SET assigned_to_id = $1,
			    assigned_at = $2
			WHERE id = $3
		`, assignment.UserID, currentTime, assignment.TicketID)

		if err != nil {
			log.Printf("%s ticket update failed: %v", logPrefix(function), err)
			return
		}
	}

	for _, assignment := range parentToUser {
		_, err = tx.Exec(ctx, `
			UPDATE panel_ticketassignmenttransaction
			SET user_id = $1
			WHERE parent_ticket_id = $2
		`, assignment.UserID, assignment.TicketID)

		if err != nil {
			log.Printf("%s transaction update failed: %v", logPrefix(function), err)
			return
		}
	}

	for profileID, ticketIDs := range ticketsPerProfile {
		_, err = tx.Exec(ctx, `
			UPDATE panel_user_profile
			SET "tickets_inHand" = "tickets_inHand" + $1
			WHERE id = $2
		`, len(ticketIDs), profileID)

		if err != nil {
			log.Printf("%s profile update failed: %v", logPrefix(function), err)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("%s commit failed: %v", logPrefix(function), err)
		return
	}

	log.Printf(
		"%s %d tickets reassigned to %d agents.",
		logPrefix(function),
		len(assignments),
		len(userQueue),
	)
}