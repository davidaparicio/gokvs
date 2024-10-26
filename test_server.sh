#!/usr/bin/env bash

echo -e "== Generation 1: The Monolith | page 121/411 ==\n"
echo "-- PUT data into key-a --"
curl -X PUT -d 'Hello, key-value store!' http://localhost:8080/v1/key-a #-v
echo "-- GET data from key-a --"
curl -X GET http://localhost:8080/v1/key-a #-v

echo -e "\n"
echo "-- PUT data into key-a --"
curl -X PUT -d 'Hello, again, key-value store!' http://localhost:8080/v1/key-a
echo "-- GET data from key-a --"
curl -X GET http://localhost:8080/v1/key-a

echo -e "\n"
echo "-- DELETE data at key-a --"
curl -X DELETE http://localhost:8080/v1/key-a #-v
echo "-- GET data from key-a --"
curl -X GET http://localhost:8080/v1/key-a

# curl localhost:8080
# curl localhost:8080/healthz
