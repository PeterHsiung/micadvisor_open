#!/bin/bash

go build run.go 
[ $? -ne 0 ] && exit 1 

go build uploadCadvisorData.go pushDatas.go mylog.go
[ $? -ne 0 ] && exit 1 

docker build -t falconcadvisor ./


