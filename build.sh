#!/bin/bash

GOOS=linux GOARCH=amd64 go build -o updater updater.go
