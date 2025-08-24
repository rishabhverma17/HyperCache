#!/bin/bash

# HyperCache Unit Test Runner
# This script runs all unit tests with coverage analysis

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

# Function to run tests with coverage
run_unit_tests() {
    local package_pattern="${1:-./tests/unit/...}"
    local coverage_file="${2:-coverage.out}"
    
    log "🧪 Running unit tests: $package_pattern"
    
    # Create test-results directory if it doesn't exist
    mkdir -p test-results
    
    # Run tests with coverage
    if go test "$package_pattern" -v -race -coverprofile="test-results/$coverage_file" -covermode=atomic -timeout=5m; then
        log "✅ Unit tests PASSED"
        return 0
    else
        error "❌ Unit tests FAILED"
        return 1
    fi
}

# Function to generate coverage report
generate_coverage_report() {
    local coverage_file="${1:-coverage.out}"
    local html_file="${2:-coverage.html}"
    
    if [[ -f "test-results/$coverage_file" ]]; then
        log "📊 Generating coverage report..."
        
        # Generate HTML coverage report
        go tool cover -html="test-results/$coverage_file" -o "test-results/$html_file"
        
        # Show coverage summary
        local coverage_percent=$(go tool cover -func="test-results/$coverage_file" | tail -1 | awk '{print $3}')
        log "📈 Total coverage: $coverage_percent"
        
        # Open coverage report in browser if requested
        if [[ "$OPEN_COVERAGE" == "true" ]]; then
            if command -v open >/dev/null 2>&1; then
                open "test-results/$html_file"
            elif command -v xdg-open >/dev/null 2>&1; then
                xdg-open "test-results/$html_file"
            else
                log "📄 Coverage report generated: test-results/$html_file"
            fi
        else
            log "📄 Coverage report generated: test-results/$html_file"
        fi
    else
        warn "No coverage file found: test-results/$coverage_file"
    fi
}

# Function to run benchmarks
run_benchmarks() {
    local package_pattern="${1:-./tests/unit/...}"
    
    log "⚡ Running benchmarks: $package_pattern"
    
    # Create benchmark results directory
    mkdir -p test-results
    
    # Run benchmarks and save results
    if go test "$package_pattern" -bench=. -benchmem -run=^$ -timeout=10m > "test-results/benchmark.txt"; then
        log "✅ Benchmarks completed"
        log "📄 Benchmark results saved: test-results/benchmark.txt"
        
        # Show summary of benchmark results
        if [[ -f "test-results/benchmark.txt" ]]; then
            log "📊 Benchmark Summary:"
            grep -E "^Benchmark" "test-results/benchmark.txt" | head -10 || true
        fi
        return 0
    else
        warn "⚠️  Some benchmarks may have failed (check test-results/benchmark.txt)"
        return 1
    fi
}

# Function to run specific test patterns
run_specific_tests() {
    local test_name="$1"
    local package_pattern="$2"
    
    log "🎯 Running specific tests: $test_name"
    
    if go test "$package_pattern" -run "$test_name" -v -race -timeout=3m; then
        log "✅ $test_name tests PASSED"
        return 0
    else
        error "❌ $test_name tests FAILED"
        return 1
    fi
}

# Function to analyze test results
analyze_results() {
    log "📈 Test Results Analysis"
    log "======================="
    
    # Coverage analysis
    if [[ -f "test-results/coverage.out" ]]; then
        log ""
        log "📊 Coverage by Package:"
        go tool cover -func="test-results/coverage.out" | grep -v "total:" | sort -k3 -nr | head -10
        
        echo ""
        local total_coverage=$(go tool cover -func="test-results/coverage.out" | tail -1)
        log "🎯 $total_coverage"
    fi
    
    # Benchmark analysis
    if [[ -f "test-results/benchmark.txt" ]]; then
        log ""
        log "⚡ Benchmark Highlights:"
        grep -E "^Benchmark.*-.*[0-9]+" "test-results/benchmark.txt" | \
        awk '{print $1 ": " $3 " " $4}' | head -5 || true
    fi
    
    log ""
    log "📁 Detailed results available in: test-results/"
}

# Function to generate structured test summary
generate_test_summary() {
    local test_output_file="test-results/test_output.log"
    
    if [[ ! -f "$test_output_file" ]]; then
        warn "No test output file found for summary generation"
        return
    fi
    
    log ""
    log "📋 HyperCache Test Results Summary"
    log "=================================="
    
    # Parse test results using simple approach
    local cache_status="NOT_RUN"
    local cluster_status="NOT_RUN" 
    local config_status="NOT_RUN"
    local filter_status="NOT_RUN"
    local network_status="NOT_RUN"
    local storage_status="NOT_RUN"
    
    # Extract component results
    if grep -q "^ok.*hypercache/tests/unit/cache" "$test_output_file"; then
        cache_status="PASS"
    elif grep -q "^FAIL.*hypercache/tests/unit/cache" "$test_output_file"; then
        cache_status="FAIL"
    fi
    
    if grep -q "^ok.*hypercache/tests/unit/cluster" "$test_output_file"; then
        cluster_status="PASS"
    elif grep -q "^FAIL.*hypercache/tests/unit/cluster" "$test_output_file"; then
        cluster_status="FAIL"
    fi
    
    if grep -q "^ok.*hypercache/tests/unit/config" "$test_output_file"; then
        config_status="PASS"
    elif grep -q "^FAIL.*hypercache/tests/unit/config" "$test_output_file"; then
        config_status="FAIL"
    fi
    
    if grep -q "^ok.*hypercache/tests/unit/filter" "$test_output_file"; then
        filter_status="PASS"
    elif grep -q "^FAIL.*hypercache/tests/unit/filter" "$test_output_file"; then
        filter_status="FAIL"
    fi
    
    if grep -q "^ok.*hypercache/tests/unit/network" "$test_output_file"; then
        network_status="PASS"
    elif grep -q "^FAIL.*hypercache/tests/unit/network" "$test_output_file"; then
        network_status="FAIL"
    fi
    
    if grep -q "^ok.*hypercache/tests/unit/storage" "$test_output_file"; then
        storage_status="PASS"
    elif grep -q "^FAIL.*hypercache/tests/unit/storage" "$test_output_file"; then
        storage_status="FAIL"
    fi
    
    # Display results with details
    local total_components=0
    local passed_components=0
    local failed_components=0
    
    # Cache Component
    if [[ "$cache_status" != "NOT_RUN" ]]; then
        total_components=$((total_components + 1))
        if [[ "$cache_status" == "PASS" ]]; then
            echo -e "${GREEN}✅ CACHE Tests: PASS${NC}"
            passed_components=$((passed_components + 1))
            # Extract cache test details
            grep "--- PASS:.*TestSessionEvictionPolicy" "$test_output_file" | head -5 | sed 's/^/    /'
        else
            echo -e "${RED}❌ CACHE Tests: FAIL${NC}"
            failed_components=$((failed_components + 1))
            grep "--- FAIL:.*TestSessionEvictionPolicy" "$test_output_file" | head -5 | sed 's/^/    /'
        fi
        echo ""
    fi
    
    # Cluster Component
    if [[ "$cluster_status" != "NOT_RUN" ]]; then
        total_components=$((total_components + 1))
        if [[ "$cluster_status" == "PASS" ]]; then
            echo -e "${GREEN}✅ CLUSTER Tests: PASS${NC}"
            passed_components=$((passed_components + 1))
            # Extract main cluster test results
            grep "--- PASS:.*TestHashRing\|--- PASS:.*TestGossipMembership\|--- PASS:.*TestSimpleCoordinator\|--- PASS:.*TestDistributedEventBus" "$test_output_file" | sed 's/^/    /'
        else
            echo -e "${RED}❌ CLUSTER Tests: FAIL${NC}"
            failed_components=$((failed_components + 1))
            grep "--- FAIL:.*TestHashRing\|--- FAIL:.*TestGossipMembership\|--- FAIL:.*TestSimpleCoordinator\|--- FAIL:.*TestDistributedEventBus" "$test_output_file" | sed 's/^/    /'
        fi
        echo ""
    fi
    
    # Config Component
    if [[ "$config_status" != "NOT_RUN" ]]; then
        total_components=$((total_components + 1))
        if [[ "$config_status" == "PASS" ]]; then
            echo -e "${GREEN}✅ CONFIG Tests: PASS${NC}"
            passed_components=$((passed_components + 1))
            grep "--- PASS:.*Configuration" "$test_output_file" | sed 's/^/    /'
        else
            echo -e "${RED}❌ CONFIG Tests: FAIL${NC}"
            failed_components=$((failed_components + 1))
            grep "--- FAIL:.*Configuration" "$test_output_file" | sed 's/^/    /'
        fi
        echo ""
    fi
    
    # Filter Component
    if [[ "$filter_status" != "NOT_RUN" ]]; then
        total_components=$((total_components + 1))
        if [[ "$filter_status" == "PASS" ]]; then
            echo -e "${GREEN}✅ FILTER Tests: PASS${NC}"
            passed_components=$((passed_components + 1))
            grep "--- PASS:.*TestCuckooFilter" "$test_output_file" | sed 's/^/    /'
        else
            echo -e "${RED}❌ FILTER Tests: FAIL${NC}"
            failed_components=$((failed_components + 1))
            # Show both pass and fail for filter
            echo "    Detailed Results:"
            grep "--- PASS:.*TestCuckooFilter\|--- FAIL:.*TestCuckooFilter" "$test_output_file" | sed 's/^/        /'
            # Show specific failure details
            if grep -q "False positive rate too high" "$test_output_file"; then
                echo -e "${YELLOW}    ⚠️  Critical Issue: False positive rate 63.1% (expected 1%)${NC}"
            fi
        fi
        echo ""
    fi
    
    # Network Component
    if [[ "$network_status" != "NOT_RUN" ]]; then
        total_components=$((total_components + 1))
        if [[ "$network_status" == "PASS" ]]; then
            echo -e "${GREEN}✅ NETWORK Tests: PASS${NC}"
            passed_components=$((passed_components + 1))
            grep "--- PASS:.*TestRESP" "$test_output_file" | sed 's/^/    /'
        else
            echo -e "${RED}❌ NETWORK Tests: FAIL${NC}"
            failed_components=$((failed_components + 1))
            grep "--- FAIL:.*TestRESP" "$test_output_file" | sed 's/^/    /'
        fi
        echo ""
    fi
    
    # Storage Component
    if [[ "$storage_status" != "NOT_RUN" ]]; then
        total_components=$((total_components + 1))
        if [[ "$storage_status" == "PASS" ]]; then
            echo -e "${GREEN}✅ STORAGE Tests: PASS${NC}"
            passed_components=$((passed_components + 1))
            grep "--- PASS:.*TestBasicStore\|--- PASS:.*TestMemoryPool" "$test_output_file" | sed 's/^/    /'
        else
            echo -e "${RED}❌ STORAGE Tests: FAIL${NC}"
            failed_components=$((failed_components + 1))
            grep "--- FAIL:.*TestBasicStore\|--- FAIL:.*TestMemoryPool" "$test_output_file" | sed 's/^/    /'
        fi
        echo ""
    fi
    
    # Summary statistics
    log "📊 Test Summary Statistics"
    log "========================="
    log "Total Components Tested: $total_components"
    echo -e "${GREEN}✅ Passed: $passed_components${NC}"
    echo -e "${RED}❌ Failed: $failed_components${NC}"
    
    if [[ $failed_components -eq 0 ]]; then
        echo -e "${GREEN}🎉 All tests passed successfully!${NC}"
    else
        local success_rate=$(( (passed_components * 100) / total_components ))
        echo -e "${YELLOW}📈 Success Rate: ${success_rate}%${NC}"
        
        # Show next steps
        if [[ $failed_components -gt 0 ]]; then
            echo ""
            echo -e "${YELLOW}🔧 Next Steps:${NC}"
            if [[ "$filter_status" == "FAIL" ]]; then
                echo "   • Fix Cuckoo filter false positive rate issue"
            fi
        fi
    fi
}

# Function to capture test output
run_unit_tests_with_summary() {
    local package_pattern="${1:-./tests/unit/...}"
    local coverage_file="${2:-coverage.out}"
    
    log "🧪 Running unit tests: $package_pattern"
    
    # Create test-results directory if it doesn't exist
    mkdir -p test-results
    
    # Run tests with coverage and capture output
    local test_output="test-results/test_output.log"
    
    if go test "$package_pattern" -v -race -coverprofile="test-results/$coverage_file" -covermode=atomic -timeout=5m 2>&1 | tee "$test_output"; then
        log "✅ Unit tests PASSED"
        generate_test_summary
        return 0
    else
        error "❌ Unit tests FAILED"
        generate_test_summary
        return 1
    fi
}

# Main execution
main() {
    log "🎯 HyperCache Unit Test Runner"
    log "============================="
    
    # Parse command line arguments
    local test_type="${1:-all}"
    local generate_coverage="${2:-true}"
    
    case "$test_type" in
        "filter"|"cuckoo-filter")
            package_pattern="./tests/unit/filter/..."
            test_name="TestCuckooFilter.*"
            coverage_file="filter_coverage.out"
            ;;
        "cache"|"cache-unit")
            package_pattern="./tests/unit/cache/..."
            test_name="Test.*"
            coverage_file="cache_coverage.out"
            ;;
        "storage"|"storage-unit")
            package_pattern="./tests/unit/storage/..."
            test_name="Test.*"
            coverage_file="storage_coverage.out"
            ;;
        "network"|"network-unit")
            package_pattern="./tests/unit/network/..."
            test_name="Test.*"
            coverage_file="network_coverage.out"
            ;;
        "cluster"|"cluster-unit")
            package_pattern="./tests/unit/cluster/..."
            test_name="Test.*"
            coverage_file="cluster_coverage.out"
            ;;
        "config"|"config-unit")
            package_pattern="./tests/unit/config/..."
            test_name="Test.*"
            coverage_file="config_coverage.out"
            ;;
        "benchmarks"|"bench")
            log "⚡ Running benchmarks only..."
            run_benchmarks "./tests/unit/..."
            exit 0
            ;;
        "quick")
            log "🏃 Running quick tests (no coverage, no benchmarks)..."
            run_specific_tests "Test.*" "./tests/unit/..."
            exit 0
            ;;
        "all"|*)
            package_pattern="./tests/unit/..."
            test_name="Test.*"
            coverage_file="coverage.out"
            ;;
    esac

    # Run the tests
    case "$test_type" in
        "all")
            # Full test suite with structured summary
            run_unit_tests_with_summary "$package_pattern" "$coverage_file"
            
            if [[ "$generate_coverage" == "true" ]]; then
                generate_coverage_report "$coverage_file"
            fi
            
            # Run benchmarks
            log ""
            run_benchmarks "$package_pattern"
            
            # Analyze results (skip since we have summary now)
            # analyze_results
            ;;
        *)
            # Specific component tests
            run_specific_tests "$test_name" "$package_pattern"
            
            if [[ "$generate_coverage" == "true" ]]; then
                run_unit_tests "$package_pattern" "$coverage_file" > /dev/null
                generate_coverage_report "$coverage_file"
            fi
            ;;
    esac

    log ""
    log "🎉 Unit testing completed successfully!"
    log ""
    log "📁 Results saved in: test-results/"
    log "   • coverage.html - HTML coverage report"
    log "   • coverage.out - Coverage data"
    log "   • benchmark.txt - Benchmark results"
}

# Help function
show_help() {
    echo "HyperCache Unit Test Runner"
    echo ""
    echo "Usage: $0 [test-type] [generate-coverage]"
    echo ""
    echo "Test Types:"
    echo "  all                  Run all unit tests with coverage and benchmarks (default)"
    echo "  filter               Run Cuckoo filter tests only"
    echo "  cache                Run cache component tests only"
    echo "  storage              Run storage component tests only"
    echo "  network              Run network component tests only"
    echo "  cluster              Run cluster component tests only"
    echo "  config               Run configuration tests only"
    echo "  benchmarks           Run benchmarks only"
    echo "  quick                Run tests without coverage or benchmarks"
    echo ""
    echo "Options:"
    echo "  generate-coverage    Set to 'false' to skip coverage report generation"
    echo ""
    echo "Environment Variables:"
    echo "  OPEN_COVERAGE=true   Automatically open coverage report in browser"
    echo ""
    echo "Examples:"
    echo "  $0                              # Run all tests with coverage"
    echo "  $0 filter                       # Run Cuckoo filter tests only"
    echo "  $0 benchmarks                   # Run benchmarks only"
    echo "  $0 quick                        # Fast test run without coverage"
    echo "  OPEN_COVERAGE=true $0           # Run tests and open coverage report"
    echo ""
    echo "Output:"
    echo "  • test-results/coverage.html - Interactive coverage report"
    echo "  • test-results/coverage.out - Coverage data"
    echo "  • test-results/benchmark.txt - Benchmark results"
}

# Check for help flag
if [[ "$1" == "-h" || "$1" == "--help" ]]; then
    show_help
    exit 0
fi

# Run main function
main "$@"
