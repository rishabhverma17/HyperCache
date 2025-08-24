#!/bin/bash

# HyperCache Integration Test Runner
# This script starts the cluster and runs integration tests

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
cd "$PROJECT_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date +'%H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date +'%H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%H:%M:%S')] ERROR: $1${NC}"
    exit 1
}

info() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')] INFO: $1${NC}"
}

# Function to cleanup persistence data before tests
cleanup_persistence() {
    log "🧹 Cleaning up persistence data before tests..."
    
    # Check if clean-persistence.sh exists
    if [[ ! -f "scripts/clean-persistence.sh" ]]; then
        warn "scripts/clean-persistence.sh not found. Skipping persistence cleanup."
        return 0
    fi

    # Make sure it's executable
    chmod +x scripts/clean-persistence.sh

    # Run cleanup script with --all flag (non-interactive mode)
    if ./scripts/clean-persistence.sh --all > /dev/null 2>&1; then
        log "✅ Persistence data cleaned up successfully"
        return 0
    else
        warn "⚠️  Failed to clean persistence data, continuing anyway..."
        return 1
    fi
}

# Function to check if cluster is running
check_cluster_running() {
    local nodes=("9080" "9081" "9082")
    for port in "${nodes[@]}"; do
        if ! curl -s "http://localhost:$port/health" > /dev/null 2>&1; then
            return 1
        fi
    done
    return 0
}

# Function to start cluster if needed
start_cluster_if_needed() {
    if check_cluster_running; then
        log "✅ Cluster is already running"
        return 0
    fi

    log "🚀 Starting HyperCache cluster for integration tests..."
    
    # Check if start-cluster.sh exists
    if [[ ! -f "scripts/start-cluster.sh" ]]; then
        error "scripts/start-cluster.sh not found. Cannot start cluster."
    fi

    # Make sure it's executable
    chmod +x scripts/start-cluster.sh

    # Start cluster
    ./scripts/start-cluster.sh cluster

    # Wait for cluster to be ready
    log "⏳ Waiting for cluster to be ready..."
    local retries=30
    local count=0
    
    while [[ $count -lt $retries ]]; do
        if check_cluster_running; then
            log "✅ Cluster is ready!"
            return 0
        fi
        
        count=$((count + 1))
        log "   Attempt $count/$retries - waiting 2 seconds..."
        sleep 2
    done

    error "❌ Cluster failed to start after $retries attempts"
}

# Arrays to track test results
declare -a TEST_NAMES=()
declare -a TEST_RESULTS=()
declare -a TEST_DETAILS=()

# Function to run specific test suites with result tracking
run_test_suite() {
    local suite_name="$1"
    local test_pattern="$2"
    
    log "🧪 Running $suite_name tests..."
    
    # Capture test output
    local test_output
    test_output=$(go test "./tests/integration/" -run "$test_pattern" -v -timeout 10m 2>&1)
    local test_exit_code=$?
    
    # Store test name and result
    TEST_NAMES+=("$suite_name")
    
    if [[ $test_exit_code -eq 0 ]]; then
        log "✅ $suite_name tests PASSED"
        TEST_RESULTS+=("PASS")
        # Extract detailed results
        local details=$(echo "$test_output" | grep -E "(PASS|FAIL|SKIP): Test" | sed 's/^[[:space:]]*//' || echo "No detailed results")
        TEST_DETAILS+=("$details")
        return 0
    else
        warn "❌ $suite_name tests FAILED"
        TEST_RESULTS+=("FAIL")
        # Extract error details
        local details=$(echo "$test_output" | grep -E "(PASS|FAIL|SKIP): Test" | sed 's/^[[:space:]]*//' || echo "Failed to extract details")
        TEST_DETAILS+=("$details")
        return 1
    fi
}

# Function to generate comprehensive test summary
generate_test_summary() {
    local total_tests=${#TEST_NAMES[@]}
    local passed_tests=0
    local failed_tests=0
    local i
    
    echo ""
    echo "=============================================="
    echo "🎯 HYPERCACHE INTEGRATION TEST SUMMARY"
    echo "=============================================="
    echo "$(date)"
    echo ""
    
    for (( i=0; i<total_tests; i++ )); do
        local name="${TEST_NAMES[$i]}"
        local result="${TEST_RESULTS[$i]}"
        local details="${TEST_DETAILS[$i]}"
        
        if [[ "$result" == "PASS" ]]; then
            echo -e "✅ ${GREEN}$name${NC}: $result"
            ((passed_tests++))
        else
            echo -e "❌ ${RED}$name${NC}: $result"
            ((failed_tests++))
        fi
        
        # Show detailed test results
        if [[ "$details" != "No detailed results" && "$details" != "Failed to extract details" ]]; then
            echo "$details" | while IFS= read -r line; do
                if [[ "$line" =~ PASS ]]; then
                    echo -e "   ${GREEN}$line${NC}"
                elif [[ "$line" =~ FAIL ]]; then
                    echo -e "   ${RED}$line${NC}"
                elif [[ "$line" =~ SKIP ]]; then
                    echo -e "   ${YELLOW}$line${NC}"
                else
                    echo "   $line"
                fi
            done
        fi
        echo ""
    done
    
    echo "=============================================="
    echo "📊 FINAL RESULTS:"
    echo "   Total Test Suites: $total_tests"
    echo -e "   ${GREEN}Passed: $passed_tests${NC}"
    echo -e "   ${RED}Failed: $failed_tests${NC}"
    
    if [[ $failed_tests -eq 0 ]]; then
        echo -e "   ${GREEN}Overall Status: ALL TESTS PASSED ✅${NC}"
    else
        echo -e "   ${RED}Overall Status: SOME TESTS FAILED ❌${NC}"
    fi
    
    local success_rate=0
    if [[ $total_tests -gt 0 ]]; then
        success_rate=$((passed_tests * 100 / total_tests))
    fi
    echo "   Success Rate: $success_rate%"
    echo "=============================================="
    echo ""
}

# Function to cleanup processes
cleanup() {
    log "🧹 Cleaning up..."
    
    # Kill any running hypercache processes
    if pgrep -f "hypercache" > /dev/null; then
        log "   Stopping HyperCache processes..."
        pkill -f "hypercache" || true
        sleep 2
    fi
    
    log "✅ Cleanup completed"
}

# Main execution
main() {
    log "🎯 HyperCache Integration Test Runner"
    log "=================================="
    
    # Parse command line arguments
    local test_type="${1:-all}"
    local skip_cluster_start="${2:-false}"
    
    case "$test_type" in
        "health"|"cluster-health")
            test_pattern="TestClusterHealth"
            ;;
        "consistency"|"data-consistency")
            test_pattern="TestDataConsistency"
            ;;
        "fault"|"fault-tolerance")
            test_pattern="TestFaultTolerance"
            ;;
        "persistence"|"persistence-recovery")
            test_pattern="TestPersistenceRecovery"
            ;;
        "cuckoo"|"cuckoo-filter")
            test_pattern="TestCuckooFilterIntegration"
            ;;
        "all"|*)
            test_pattern="Test.*"
            ;;
    esac

    # Setup trap for cleanup
    trap cleanup EXIT

    # Clean persistence data before starting tests
    cleanup_persistence

    # Start cluster if needed
    if [[ "$skip_cluster_start" != "true" ]]; then
        start_cluster_if_needed
        
        # Wait 10 seconds for cluster to fully stabilize after startup
        log "⏳ Waiting 10 seconds for cluster to fully stabilize..."
        sleep 10
        log "✅ Cluster stabilization wait completed"
    else
        log "⏭️  Skipping cluster start (assuming cluster is already running)"
        if ! check_cluster_running; then
            warn "Cluster doesn't appear to be running, but proceeding anyway..."
        fi
    fi

    # Create logs directory if it doesn't exist
    mkdir -p logs

    # Run tests based on type
    local overall_success=true
    case "$test_type" in
        "all")
            log "🔄 Running ALL integration tests in single terminal..."
            echo ""
            
            # Run each test suite and track results
            run_test_suite "Cluster Health" "TestClusterHealth" || overall_success=false
            echo ""
            run_test_suite "Data Consistency" "TestDataConsistency" || overall_success=false
            echo ""
            run_test_suite "Cuckoo Filter Integration" "TestCuckooFilterIntegration" || overall_success=false
            echo ""
            
            # Note about manual tests
            log "ℹ️  Note: Fault tolerance and persistence tests require manual node management"
            log "   You can run them separately with: $0 fault-tolerance"
            log "   You can run them separately with: $0 persistence"
            ;;
        *)
            run_test_suite "$(echo $test_type | tr '[:lower:]' '[:upper:]')" "$test_pattern" || overall_success=false
            ;;
    esac

    # Generate comprehensive summary
    generate_test_summary
    
    if [[ "$overall_success" == "true" ]]; then
        log "🎉 All executed integration tests completed successfully!"
        exit 0
    else
        warn "⚠️  Some integration tests failed. Check the summary above for details."
        exit 1
    fi
}

# Help function
show_help() {
    echo "HyperCache Integration Test Runner"
    echo ""
    echo "Usage: $0 [test-type] [skip-cluster-start]"
    echo ""
    echo "Test Types:"
    echo "  all                  Run all integration tests (default)"
    echo "  health               Run cluster health tests only"
    echo "  consistency          Run data consistency tests only"
    echo "  fault-tolerance      Run fault tolerance tests only (requires manual setup)"
    echo "  persistence          Run persistence recovery tests only (requires manual setup)"
    echo "  cuckoo-filter        Run Cuckoo filter integration tests only"
    echo ""
    echo "Options:"
    echo "  skip-cluster-start   Set to 'true' to skip automatic cluster startup"
    echo ""
    echo "Examples:"
    echo "  $0                              # Run all tests"
    echo "  $0 health                       # Run health tests only"
    echo "  $0 cuckoo-filter                # Run Cuckoo filter tests only"
    echo "  $0 all true                     # Run all tests, skip cluster start"
    echo ""
    echo "Prerequisites:"
    echo "  • Go 1.23+ installed"
    echo "  • HyperCache built: go build -o bin/hypercache cmd/hypercache/main.go"
    echo "  • For manual tests: Start nodes individually as documented"
}

# Check for help flag
if [[ "$1" == "-h" || "$1" == "--help" ]]; then
    show_help
    exit 0
fi

# Run main function
main "$@"
