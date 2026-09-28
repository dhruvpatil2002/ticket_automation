package db

import (
	"context"
	"log"
	"time"
)

func InactiveUsers(ctx context.Context) {
	const function = "inactive_users"

	tx, err := DB.Begin(ctx)
	if err != nil {
		log.Printf("%s unable to begin transaction: %v", logPrefix(function), err)
		return
	}
	defer tx.Rollback(ctx)

	currentTime := now()
	compare15Min := currentTime.Add(-15 * time.Minute)
	compare5Min := currentTime.Add(-5 * time.Minute)

	rows, err := tx.Query(ctx, `
		SELECT up.id, up.user_id, t.id, s.bu_id, t.site_id
		FROM panel_user_profile up
		JOIN auth_user u ON up.user_id = u.id
		LEFT JOIN panel_ticket t
			ON t.assigned_to_id = up.user_id
			AND t.is_parent = TRUE
			AND t.closed = FALSE
		LEFT JOIN panel_site s ON s.id = t.site_id
		WHERE up.last_active < $1
		  AND s.enterprise_id != 34
		  AND up.role = 'agent'
		  AND u.username != 'no_agent'
	`, compare15Min)
	if err != nil {
		log.Printf("%s inactive user query failed: %v", logPrefix(function), err)
		return
	}
	defer rows.Close()

	profileIDs := make(map[int64]struct{})
	ticketList := make([]Ticket, 0)
	ticketBUMap := make(map[int64]int64)
	ticketOldUserMap := make(map[int64]int64)
	buSet := make(map[int64]struct{})

	for rows.Next() {
		var profileID, userID int64
		var ticketID, buID, siteID *int64

		if err := rows.Scan(
			&profileID,
			&userID,
			&ticketID,
			&buID,
			&siteID,
		); err != nil {
			log.Printf("%s scan failed: %v", logPrefix(function), err)
			return
		}

		profileIDs[profileID] = struct{}{}

		if ticketID != nil {
			ticketList = append(ticketList, Ticket{
				ID:     *ticketID,
				SiteID: siteID,
			})

			ticketOldUserMap[*ticketID] = userID

			if buID != nil {
				ticketBUMap[*ticketID] = *buID
				buSet[*buID] = struct{}{}
			}
		}
	}

	if len(profileIDs) == 0 {
		log.Printf("%s No inactive agents found.", logPrefix(function))
		return
	}

	profileIDList := make([]int64, 0, len(profileIDs))
	for profileID := range profileIDs {
		profileIDList = append(profileIDList, profileID)
	}

	_, err = tx.Exec(ctx, `
		UPDATE panel_user_profile
		SET on_shift_end = TRUE
		WHERE id = ANY($1)
	`, profileIDList)
	if err != nil {
		log.Printf("%s failed to mark users as shift-ended: %v", logPrefix(function), err)
		return
	}

	if len(ticketList) == 0 {
		if err := tx.Commit(ctx); err != nil {
			log.Printf("%s commit failed: %v", logPrefix(function), err)
		}
		log.Printf("%s No tickets found for inactive users.", logPrefix(function))
		return
	}

	buIDs := make([]int64, 0, len(buSet))
	for buID := range buSet {
		buIDs = append(buIDs, buID)
	}

	rows, err = tx.Query(ctx, `
		SELECT up.id, up.user_id, pupb.business_unit_id
		FROM panel_user_profile up
		JOIN auth_user u ON up.user_id = u.id
		JOIN panel_user_profile_bu pupb
			ON pupb.user_profile_id = up.id
		JOIN panel_business_unit pbu
			ON pbu.id = pupb.business_unit_id
		WHERE up.on_break = FALSE
		  AND up.on_shift_end = FALSE
		  AND up.role = 'agent'
		  AND pupb.business_unit_id = ANY($1)
		  AND up.last_active >= $2
		  AND pbu.enterprise_id != 34
	`, buIDs, compare5Min)
	if err != nil {
		log.Printf("%s available agent query failed: %v", logPrefix(function), err)
		return
	}

	agentsByBU := make(map[int64][]Agent)

	for rows.Next() {
		var agent Agent

		if err := rows.Scan(
			&agent.ProfileID,
			&agent.UserID,
			&agent.BUID,
		); err != nil {
			rows.Close()
			log.Printf("%s available agent scan failed: %v", logPrefix(function), err)
			return
		}

		agentsByBU[agent.BUID] = append(agentsByBU[agent.BUID], agent)
	}
	rows.Close()

	if len(agentsByBU) == 0 {
		rows, err = tx.Query(ctx, `
			SELECT up.id, up.user_id, pupb.business_unit_id
			FROM panel_user_profile up
			JOIN auth_user u ON up.user_id = u.id
			JOIN panel_user_profile_bu pupb
				ON pupb.user_profile_id = up.id
			WHERE u.username = 'no_agent'
			LIMIT 1
		`)
		if err != nil {
			log.Printf("%s fallback agent query failed: %v", logPrefix(function), err)
			return
		}

		for rows.Next() {
			var agent Agent

			if err := rows.Scan(
				&agent.ProfileID,
				&agent.UserID,
				&agent.BUID,
			); err != nil {
				rows.Close()
				log.Printf("%s fallback agent scan failed: %v", logPrefix(function), err)
				return
			}

			agentsByBU[agent.BUID] = append(agentsByBU[agent.BUID], agent)
		}
		rows.Close()
	}

	if len(agentsByBU) == 0 {
		log.Printf("%s No available agents or fallback found.", logPrefix(function))
		return
	}

	roundRobinIndex := make(map[int64]int)
	parentToUser := make(map[int64]Agent)
	profileAssignmentCount := make(map[int64]int)
	assignments := make(map[int64]Agent)

	for _, ticket := range ticketList {
		buID, exists := ticketBUMap[ticket.ID]
		if !exists {
			continue
		}

		availableAgents := agentsByBU[buID]
		if len(availableAgents) == 0 {
			continue
		}

		index := roundRobinIndex[buID]
		agent := availableAgents[index]

		roundRobinIndex[buID] =
			(index + 1) % len(availableAgents)

		assignments[ticket.ID] = agent
		parentToUser[ticket.ID] = agent
		profileAssignmentCount[agent.ProfileID]++
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

		if agent, exists := parentToUser[fromID]; exists {
			assignments[toID] = agent
		}
	}
	childRows.Close()

	for ticketID, agent := range assignments {
		_, err = tx.Exec(ctx, `
			UPDATE panel_ticket
			SET assigned_to_id = $1,
			    assigned_at = $2
			WHERE id = $3
		`, agent.UserID, currentTime, ticketID)

		if err != nil {
			log.Printf("%s ticket update failed: %v", logPrefix(function), err)
			return
		}
	}

	for ticketID, agent := range parentToUser {
		_, err = tx.Exec(ctx, `
			UPDATE panel_ticketassignmenttransaction
			SET user_id = $1
			WHERE parent_ticket_id = $2
		`, agent.UserID, ticketID)

		if err != nil {
			log.Printf("%s transaction update failed: %v", logPrefix(function), err)
			return
		}
	}

	for profileID, count := range profileAssignmentCount {
		_, err = tx.Exec(ctx, `
			UPDATE panel_user_profile
			SET "tickets_inHand" = "tickets_inHand" + $1
			WHERE id = $2
		`, count, profileID)

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
		"%s Reassigned %d parent tickets.",
		logPrefix(function),
		len(parentToUser),
	)

	_ = ticketOldUserMap
}