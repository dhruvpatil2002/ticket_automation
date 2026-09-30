package worker

import (
	"context"
	"fmt"
	"sync"

	"ticket_automation/internal/db"
)

func RunInParallel(ctx context.Context) {
	var wg sync.WaitGroup

	workers := []func(context.Context){
		db.GetClosedTransactions,
		db.DistributeNoAgent,
		db.InactiveUsers,
		func(ctx context.Context) {
			if err := db.ExpiredTickets(ctx); err != nil {
				fmt.Printf("ExpiredTickets error: %v\n", err)
			}
		},
	}

	wg.Add(len(workers))

	for _, worker := range workers {
		go func(fn func(context.Context)) {
			defer wg.Done()

			defer func() {
				if recovered := recover(); recovered != nil {
					fmt.Printf("Worker panic: %v\n", recovered)
				}
			}()

			fn(ctx)
		}(worker)
	}

	wg.Wait()
}
