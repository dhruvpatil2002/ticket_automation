package models

type Ticket struct {
	ID     int64
	SiteID *int64
}

type Agent struct {
	ProfileID int64
	UserID    int64
	BUID      int64
}

type Assignment struct {
	TicketID  int64
	UserID    int64
	ProfileID int64
}

type UserKey struct {
	ProfileID int64
	UserID    int64
}

type ExpirationResult struct {
	SummaryMessage string
	ClosedCount    int64
}
