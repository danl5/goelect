#!/bin/bash

node_addrs=("127.0.0.1:9981" "127.0.0.1:9982" "127.0.0.1:9983")
peers="127.0.0.1:9981,127.0.0.1:9982,127.0.0.1:9983"

elect_binary="node"
pid_file="./node_pids"

start_nodes() {
    echo "Starting nodes..."
    for node_addr in "${node_addrs[@]}"; do
        echo "Starting node with address $node_addr"
        log_file="${node_addr//:/_}.log"
        ./$elect_binary --nodeaddr=$node_addr --peers=$peers 2>&1 | tee "$log_file" &
        echo $! >> $pid_file
    done
    echo "All nodes started."
}

stop_nodes() {
    echo "Stopping nodes..."
    if [ -f $pid_file ]; then
        while read -r pid; do
            echo "Stopping process $pid"
            kill -9 $pid
        done < $pid_file
        rm $pid_file
        echo "All nodes stopped."
    else
        echo "No nodes are running."
    fi
}

show_help() {
    echo "Usage: $0 {start|stop}"
}

case "$1" in
    start)
        start_nodes
        ;;
    stop)
        stop_nodes
        ;;
    *)
        show_help
        ;;
esac