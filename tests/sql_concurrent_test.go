package tests

import (
	"fmt"
	"net"
	"sync"
	"testing"
)

// TestSQLConcurrent_MultipleClients verifies concurrent client connections executing isolated statements against the wire server daemon.
func TestSQLConcurrent_MultipleClients(t *testing.T) {
	initConn, pgAddr, cleanup := setupIntegrationServer(t)
	defer cleanup()

	performHandshake(t, initConn)
	_, _ = sendQuery(initConn, "CREATE DATABASE concurrent_db; USE concurrent_db;")
	_, _ = sendQuery(initConn, "CREATE TABLE client_log (client_id INT PRIMARY KEY, tag VARCHAR(20));")
	initConn.Close()

	const concurrentWorkers = 5
	var waitGroup sync.WaitGroup
	waitGroup.Add(concurrentWorkers)

	for workerIdx := 0; workerIdx < concurrentWorkers; workerIdx++ {
		clientID := workerIdx + 1
		go func(id int) {
			defer waitGroup.Done()

			clientConn, err := net.Dial("tcp", pgAddr)
			if err != nil {
				t.Errorf("worker %d TCP connection failed: %v", id, err)
				return
			}
			defer clientConn.Close()

			performHandshake(t, clientConn)
			_, _ = sendQuery(clientConn, "USE concurrent_db;")

			insertSQL := fmt.Sprintf("INSERT INTO client_log VALUES (%d, 'worker-%d');", id, id)
			insertRes, err := sendQuery(clientConn, insertSQL)
			if err != nil || len(insertRes) == 0 || insertRes[0].Err != nil {
				t.Errorf("worker %d INSERT failed: err=%v, res=%+v", id, err, insertRes)
				return
			}

			selectSQL := fmt.Sprintf("SELECT tag FROM client_log WHERE client_id = %d;", id)
			selectRes, err := sendQuery(clientConn, selectSQL)
			if err != nil || len(selectRes) == 0 || selectRes[0].Err != nil || len(selectRes[0].Rows) != 1 {
				t.Errorf("worker %d SELECT failed: err=%v, res=%+v", id, err, selectRes)
				return
			}

			expectedTag := fmt.Sprintf("worker-%d", id)
			if selectRes[0].Rows[0][0] != expectedTag {
				t.Errorf("worker %d expected tag %s, received %s", id, expectedTag, selectRes[0].Rows[0][0])
			}
		}(clientID)
	}

	waitGroup.Wait()
}
