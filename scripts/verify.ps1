# ====================================================================
# Ticket Automation Verification Script
# ====================================================================

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host " TICKET AUTOMATION DATABASE STATUS CHECK" -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

$dbCommand = {
    param($query)
    $query | docker exec -i ticket_postgres psql -U ticket_automation -d ticket_automation
}

Write-Host "`n[1] Check Tickets (Assignments, Status, and Comments):" -ForegroundColor Yellow
& $dbCommand @"
SELECT 
    id, 
    site_id, 
    assigned_to_id, 
    is_parent, 
    closed, 
    "isAccepted", 
    comments 
FROM panel_ticket 
ORDER BY id;
"@

Write-Host "`n[2] Check Agent Profiles (Shift End, Tickets in Hand, Last Active):" -ForegroundColor Yellow
& $dbCommand @"
SELECT 
    id, 
    user_id, 
    role, 
    on_break, 
    on_shift_end, 
    "tickets_inHand", 
    to_char(last_active, 'YYYY-MM-DD HH24:MI:SS') as last_active 
FROM panel_user_profile 
ORDER BY id;
"@

Write-Host "`n[3] Check Active Transactions:" -ForegroundColor Yellow
& $dbCommand @"
SELECT 'normal' as source, id, parent_ticket_id, user_id FROM panel_ticketassignmenttransaction
UNION ALL
SELECT 'safebox' as source, id, parent_ticket_id, user_id FROM panel_ticketassignmenttransaction_safebox
UNION ALL
SELECT 'safeboxai' as source, id, parent_ticket_id, user_id FROM panel_ticketassignmenttransaction_safeboxai
ORDER BY source, id;
"@
