#!/usr/bin/env bash

FuzzFUNC="Fuzz" # "FuzzReverse"
#nix-shell -p gosec govulncheck

if ! command -v golangci-lint  &> /dev/null
then
  echo "golangci-lint required but it's not installed. Skipping."
else
  echo "Let's Lint, first.."
  golangci-lint run #./...
fi

echo "Let's Test"
go test -v ./... -coverprofile=coverage.out
## go help testflag: The idiomatic way to disable test caching explicitly
## is to use -count=1.
# go test -v ./... -coverprofile=coverage.out -count=1

echo "Let's Test (race detector)"
go test -race ./...
# https://go.dev/doc/articles/race_detector

echo "Let's Mutate (beware mutants)"
# https://github.com/avito-tech/go-mutesting
if ! command -v go-mutesting &> /dev/null
then
  echo "go-mutesting required but it's not installed. Skipping."
  # go install github.com/avito-tech/go-mutesting/... | go get -t -v github.com/avito-tech/go-mutesting/...
else
  go-mutesting ./...
  # The mutation score is 0.305732 (48 passed, 109 failed, 11 duplicated, 0 skipped, total is 157) [in 3minutes]
  open -a firefox report.json
  #open report.json
fi
# https://github.com/go-gremlins/gremlins
# https://gremlins.dev/0.5/install/
if ! command -v gremlins &> /dev/null
then
  echo "go-gremlins required but it's not installed. Skipping."
  # brew tap go-gremlins/tap
  # brew install gremlins
else
  gremlins unleash .
  # Mutation testing completed in 4 seconds 213 milliseconds
  # Killed: 18, Lived: 0, Not covered: 12
  # Timed out: 2, Not viable: 0, Skipped: 0
  # Test efficacy: 100.00%
  # Mutator coverage: 60.00%
fi

echo "Let's Fuzz" #cannot use -fuzz flag with multiple packages
go test ./internal -fuzz ${FuzzFUNC} -fuzztime 15s

echo "Let's Bench"
# Hardware bench with https://github.com/criteo/hwbench
# DB benchmark with https://github.com/doctolib/pg-index-benchmark
go test -v ./... -run=^$ -bench . -benchmem -benchtime=3s ./

echo "Finally, the security..."
if ! command -v gosec &> /dev/null
then
  echo "gosec required but it's not installed. Skipping."
  # brew install gosec
else
  echo "Let's Gosec"
  gosec ./...
  # golangci-lint run -E gosec --verbose ./...
  # gosec -no-fail -fmt sarif -out results.sarif ./...
fi

if ! command -v govulncheck &> /dev/null
then
  echo "govulncheck required but it's not installed. Skipping."
  # go install golang.org/x/vuln/cmd/govulncheck@latest
else
  echo "Let's Govulncheck"
  govulncheck ./...
fi