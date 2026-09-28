package worker

import (
	"context"
	"log"
	"sync"

	 "ticket_automation/internal/db"
)	


func RunInParallel(ctx context.Context) {
	var wg sync.WaitGroup

	workers := []func(context.Context){
		db.GetClosedTransactions,
		db.DistributeNoAgent,
		db.InactiveUsers,
		db.ExpiredTickets,
	}

	wg.Add(len(workers))

	for _, worker := range workers {
		go func(fn func(context.Context)) {
			defer wg.Done()

			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf(
						"Worker panic: %v",
						recovered,
					)
				}
			}()

			fn(ctx)
		}(worker)
	}

	wg.Wait()
}