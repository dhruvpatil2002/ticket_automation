package db

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ISTLocation = "Asia/Kolkata"
	NoAgentID   = 602
)

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

var (
	DB  *pgxpool.Pool
	loc *time.Location
)

func InitLocation() error {
	var err error
	loc, err = time.LoadLocation(ISTLocation)
	return err
}

func now() time.Time {
	if loc == nil {
		return time.Now()
	}
	return time.Now().In(loc)
}

func logPrefix(function string) string {
	return fmt.Sprintf("%s || %s ||", now().Format(time.RFC3339), function)
}
