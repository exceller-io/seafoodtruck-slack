#!/usr/bin/env sh
CMD=$1

echo "Command :" $CMD

case $CMD in
    "start")
        echo "Starting Golang application"
        exec ./bot
    ;;
esac