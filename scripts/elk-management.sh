#!/bin/bash

# ELK Stack Management Script for HyperCache
# Usage: ./elk-management.sh [command]

set -e

COMPOSE_FILE="docker-compose.cluster.yml"
ES_URL="localhost:9200"
GRAFANA_URL="localhost:3000"
GRAFANA_CREDS="admin:admin123"

show_help() {
    echo "HyperCache ELK Stack Management"
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  status          - Show status of all services"
    echo "  health          - Check health of ELK stack"
    echo "  logs-count      - Count total logs in Elasticsearch"
    echo "  logs-recent     - Show 10 most recent logs"
    echo "  logs-errors     - Show error-level logs"
    echo "  cuckoo-logs     - Show Cuckoo filter operations (DEBUG level)"
    echo "  test-cuckoo     - Test Cuckoo filter and show logs"
    echo "  clean-logs      - Delete all log data (keep HyperCache data)"
    echo "  clean-all       - Delete all data and volumes"
    echo "  restart-elk     - Restart ELK stack only"
    echo "  restart-all     - Restart entire cluster"
    echo "  debug-filebeat  - Show Filebeat debugging info"
    echo "  debug-es        - Show Elasticsearch debugging info"
    echo "  debug-grafana   - Show Grafana debugging info"
    echo "  fresh-start     - Complete clean restart"
    echo ""
}

status() {
    echo "=== Container Status ==="
    docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep hypercache
}

health() {
    echo "=== Elasticsearch Health ==="
    curl -s "$ES_URL/_cluster/health?pretty" | jq '{status: .status, number_of_nodes: .number_of_nodes, active_primary_shards: .active_primary_shards}'
    
    echo -e "\n=== Grafana Health ==="
    if curl -s -u "$GRAFANA_CREDS" "$GRAFANA_URL/api/health" | jq . 2>/dev/null; then
        echo "Grafana is healthy"
    else
        echo "Grafana health check failed"
    fi
    
    echo -e "\n=== Log Count ==="
    log_count=$(curl -s "$ES_URL/hypercache-docker-logs-*/_search?size=0" | jq '.hits.total.value')
    echo "Total logs in Elasticsearch: $log_count"
}

logs_count() {
    count=$(curl -s "$ES_URL/hypercache-docker-logs-*/_search?size=0" | jq '.hits.total.value')
    echo "Total logs: $count"
}

logs_recent() {
    echo "=== 10 Most Recent Logs ==="
    curl -s "$ES_URL/hypercache-docker-logs-*/_search?sort=@timestamp:desc&size=10" | \
    jq -r '.hits.hits[]._source | "\(.["@timestamp"]) [\(.level // "INFO")] \(.node_id // "unknown"): \(.message // .log)"' | \
    head -10
}

logs_errors() {
    echo "=== Error Level Logs ==="
    curl -s "$ES_URL/hypercache-docker-logs-*/_search?q=level:ERROR&sort=@timestamp:desc&size=10" | \
    jq -r '.hits.hits[]._source | "\(.["@timestamp"]) [\(.level)] \(.node_id): \(.message // .log)"'
}

cuckoo_filter_logs() {
    echo "=== Cuckoo Filter Operations (DEBUG level) ==="
    echo "Recent filter operations:"
    curl -s "$ES_URL/hypercache-docker-logs-*/_search?q=component:filter&sort=@timestamp:desc&size=10" | \
    jq -r '.hits.hits[]._source | "\(.["@timestamp"]) [\(.action)] \(.fields.key): \(.message)"'
    
    echo -e "\nNegative lookups (early rejections):"
    curl -s "$ES_URL/hypercache-docker-logs-*/_search?q=action:negative_lookup&sort=@timestamp:desc&size=5" | \
    jq -r '.hits.hits[]._source | "\(.["@timestamp"]) REJECTED: \(.fields.key)"'
    
    echo -e "\nPositive lookups (possible matches):"
    curl -s "$ES_URL/hypercache-docker-logs-*/_search?q=action:positive_lookup&sort=@timestamp:desc&size=5" | \
    jq -r '.hits.hits[]._source | "\(.["@timestamp"]) POSSIBLE: \(.fields.key)"'
}

test_cuckoo_filter() {
    echo "=== Testing Cuckoo Filter Operations ==="
    echo "1. Adding test key..."
    curl -X PUT http://localhost:9080/api/cache/cuckoo-test -H "Content-Type: application/json" -d '{"value":"test"}'
    
    echo -e "\n2. Testing cache hit (should show positive_lookup)..."
    curl http://localhost:9080/api/cache/cuckoo-test
    
    echo -e "\n3. Testing cache miss (should show negative_lookup)..."
    curl http://localhost:9080/api/cache/definitely-missing-key
    
    echo -e "\n4. Waiting for logs to be ingested..."
    sleep 3
    
    echo -e "\n5. Recent filter activity:"
    docker logs hypercache-node1 --tail 10 | grep -E "(negative_lookup|positive_lookup|filter)" || echo "No filter logs found (check DEBUG level is enabled)"
}

clean_logs() {
    echo "=== Cleaning Log Data Only ==="
    read -p "This will delete all log data. Continue? (y/N): " confirm
    if [[ $confirm == [yY] ]]; then
        echo "Deleting Elasticsearch data streams..."
        curl -X DELETE "$ES_URL/_data_stream/hypercache-docker-logs" 2>/dev/null || true
        curl -X DELETE "$ES_URL/hypercache-docker-logs-*" 2>/dev/null || true
        
        echo "Clearing local log files..."
        rm -f logs/*.log
        
        echo "Restarting Filebeat..."
        docker restart hypercache-filebeat
        
        echo "Log data cleaned. Fresh log collection started."
    else
        echo "Cancelled."
    fi
}

clean_all() {
    echo "=== Complete Data Wipe ==="
    read -p "This will delete ALL data including HyperCache persistence. Continue? (y/N): " confirm
    if [[ $confirm == [yY] ]]; then
        echo "Stopping all containers..."
        docker-compose -f "$COMPOSE_FILE" down --volumes --remove-orphans
        
        echo "Removing data directories..."
        sudo rm -rf data/elasticsearch/* logs/* data/node-*/* 2>/dev/null || true
        
        echo "Starting fresh cluster..."
        docker-compose -f "$COMPOSE_FILE" up -d
        
        echo "Complete clean restart initiated."
    else
        echo "Cancelled."
    fi
}

restart_elk() {
    echo "=== Restarting ELK Stack ==="
    docker restart hypercache-elasticsearch
    sleep 10
    docker restart hypercache-filebeat
    docker restart hypercache-grafana
    echo "ELK stack restarted."
}

restart_all() {
    echo "=== Restarting Entire Cluster ==="
    docker-compose -f "$COMPOSE_FILE" restart
    echo "Cluster restarted."
}

debug_filebeat() {
    echo "=== Filebeat Debug Info ==="
    echo "Container status:"
    docker ps | grep filebeat
    
    echo -e "\nLast 20 Filebeat log lines:"
    docker logs hypercache-filebeat --tail 20
    
    echo -e "\nFilebeat registry:"
    docker exec hypercache-filebeat ls -la /usr/share/filebeat/data/registry/filebeat/ 2>/dev/null || echo "Registry not accessible"
    
    echo -e "\nConnection test to Elasticsearch:"
    docker exec hypercache-filebeat curl -s hypercache-elasticsearch:9200/_cluster/health 2>/dev/null | jq '.status' || echo "Connection failed"
}

debug_es() {
    echo "=== Elasticsearch Debug Info ==="
    echo "Cluster health:"
    curl -s "$ES_URL/_cluster/health?pretty"
    
    echo -e "\nIndices:"
    curl -s "$ES_URL/_cat/indices/hypercache*?v"
    
    echo -e "\nData streams:"
    curl -s "$ES_URL/_data_stream?pretty"
    
    echo -e "\nRecent index activity:"
    curl -s "$ES_URL/_cat/indices/hypercache*?v&s=creation.date:desc" | head -5
}

debug_grafana() {
    echo "=== Grafana Debug Info ==="
    echo "Container status:"
    docker ps | grep grafana
    
    echo -e "\nGrafana health:"
    curl -s -u "$GRAFANA_CREDS" "$GRAFANA_URL/api/health" | jq . 2>/dev/null || echo "Health check failed"
    
    echo -e "\nDatasources:"
    curl -s -u "$GRAFANA_CREDS" "$GRAFANA_URL/api/datasources" | jq '.[] | {name: .name, type: .type, url: .url}'
    
    echo -e "\nConnection to Elasticsearch from Grafana:"
    docker exec hypercache-grafana curl -s hypercache-elasticsearch:9200/_cluster/health | jq '.status' 2>/dev/null || echo "Connection failed"
}

fresh_start() {
    echo "=== Fresh Start ==="
    read -p "This will stop all containers and remove volumes. Continue? (y/N): " confirm
    if [[ $confirm == [yY] ]]; then
        docker-compose -f "$COMPOSE_FILE" down --volumes
        docker-compose -f "$COMPOSE_FILE" up -d
        echo "Fresh start completed."
    else
        echo "Cancelled."
    fi
}

# Main command handling
case "${1:-}" in
    status) status ;;
    health) health ;;
    logs-count) logs_count ;;
    logs-recent) logs_recent ;;
    logs-errors) logs_errors ;;
    cuckoo-logs) cuckoo_filter_logs ;;
    test-cuckoo) test_cuckoo_filter ;;
    clean-logs) clean_logs ;;
    clean-all) clean_all ;;
    restart-elk) restart_elk ;;
    restart-all) restart_all ;;
    debug-filebeat) debug_filebeat ;;
    debug-es) debug_es ;;
    debug-grafana) debug_grafana ;;
    fresh-start) fresh_start ;;
    help|--help|-h) show_help ;;
    "") show_help ;;
    *) echo "Unknown command: $1"; show_help; exit 1 ;;
esac
