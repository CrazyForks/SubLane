package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// cleanup_pending_allocations cleans up allocation entries stuck in pending state
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run cleanup_pending_allocations.go <path-to-sublane.db>")
		os.Exit(1)
	}

	dbPath := os.Args[1]

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer conn.Close()

	ctx := context.Background()
	now := time.Now().Unix()

	// Find pending entries older than 10 minutes
	rows, err := conn.QueryContext(ctx, `
		SELECT request_id, scheme_id, user_id, started_at
		FROM allocation_entries
		WHERE state = 'pending'
		AND started_at < ?
	`, now-600)
	if err != nil {
		log.Fatalf("Failed to query pending entries: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var requestID string
		var schemeID, userID, startedAt int64

		if err := rows.Scan(&requestID, &schemeID, &userID, &startedAt); err != nil {
			log.Printf("Failed to scan row: %v", err)
			continue
		}

		age := time.Duration(now-startedAt) * time.Second
		fmt.Printf("Cleaning up pending entry: request_id=%s, scheme_id=%d, user_id=%d, age=%v\n",
			requestID, schemeID, userID, age)

		// Settle with zero usage
		_, err = conn.ExecContext(ctx, `
			UPDATE allocation_entries
			SET state = 'active',
			    input = 0,
			    output = 0,
			    cached = 0,
			    settled_at = ?
			WHERE request_id = ?
		`, now, requestID)

		if err != nil {
			log.Printf("Failed to settle entry %s: %v", requestID, err)
			continue
		}

		count++
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	fmt.Printf("\nCleaned up %d pending allocation entries\n", count)
}
