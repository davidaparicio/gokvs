package main

import (
	"flag"
	"log"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	flag.Parse()
	setup()
	v := m.Run()
	teardown()
	os.Exit(v)
}

func TestNoting(t *testing.T) {
	t.Log("TestNoting-Start")
	defer t.Log("TestNoting-End")
}

func TestNoting2(t *testing.T) {
	t.Log("TestNoting2-Start")
	defer t.Log("TestNoting2-End")
}

func setup() {
	log.Printf("SETUP")
}

func teardown() {
	log.Printf("TEARDOWN")
}
