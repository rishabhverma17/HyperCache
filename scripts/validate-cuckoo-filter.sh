#!/bin/bash

# Cuckoo Filter Performance Validator
# Extracts and validates FPR from test output

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
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

# Business requirement
BUSINESS_REQUIREMENT_PCT=0.1

validate_cuckoo_filter_fpr() {
    log "🧪 Validating Cuckoo Filter False Positive Rate"
    echo ""
    
    # Create results directory
    mkdir -p test-results
    
    # Run Cuckoo Filter tests with verbose output
    info "Running Cuckoo Filter tests..."
    if go test -v ./tests/unit/filter/... > test-results/cuckoo-validation.txt 2>&1; then
        log "✅ Cuckoo Filter tests completed successfully"
    else
        warn "Some tests may have failed, but continuing with FPR extraction..."
    fi
    
    echo ""
    info "📊 Extracting False Positive Rate from test results..."
    
    # Extract FPR from test output
    if grep -q "False positive rate:" test-results/cuckoo-validation.txt; then
        # Get the FPR line
        FPR_LINE=$(grep "False positive rate:" test-results/cuckoo-validation.txt | head -1)
        echo "   Raw output: $FPR_LINE"
        
        # Extract numerical values
        ACTUAL_FPR=$(echo "$FPR_LINE" | sed -E 's/.*False positive rate: ([0-9.]+).*/\1/')
        EXPECTED_FPR=$(echo "$FPR_LINE" | sed -E 's/.*expected: ([0-9.]+).*/\1/')
        
        # Convert to percentage
        if command -v bc >/dev/null 2>&1; then
            ACTUAL_PCT=$(echo "scale=4; $ACTUAL_FPR * 100" | bc -l)
            EXPECTED_PCT=$(echo "scale=4; $EXPECTED_FPR * 100" | bc -l)
        else
            # Fallback without bc
            ACTUAL_PCT=$(echo "$ACTUAL_FPR * 100" | awk '{print $1 * $3}')
            EXPECTED_PCT=$(echo "$EXPECTED_FPR * 100" | awk '{print $1 * $3}')
        fi
        
        echo ""
        log "📈 Cuckoo Filter Performance Results:"
        echo "   🎯 Actual FPR: ${ACTUAL_PCT}%"
        echo "   📋 Expected FPR: ${EXPECTED_PCT}%"
        echo "   🏢 Business Requirement: ≤${BUSINESS_REQUIREMENT_PCT}%"
        echo ""
        
        # Validate against business requirement
        if command -v bc >/dev/null 2>&1; then
            MEETS_REQUIREMENT=$(echo "$ACTUAL_PCT <= $BUSINESS_REQUIREMENT_PCT" | bc -l)
        else
            # Fallback comparison
            MEETS_REQUIREMENT=$(awk -v a="$ACTUAL_PCT" -v b="$BUSINESS_REQUIREMENT_PCT" 'BEGIN{print (a <= b)}')
        fi
        
        if [ "$MEETS_REQUIREMENT" -eq 1 ]; then
            log "✅ SUCCESS: Cuckoo Filter achieves ${ACTUAL_PCT}% FPR"
            log "✅ EXCEEDS business requirement of ≤${BUSINESS_REQUIREMENT_PCT}%"
            
            # Calculate improvement factor
            if command -v bc >/dev/null 2>&1; then
                IMPROVEMENT_FACTOR=$(echo "scale=1; $BUSINESS_REQUIREMENT_PCT / $ACTUAL_PCT" | bc -l)
                echo "   🚀 Performance is ${IMPROVEMENT_FACTOR}x better than required!"
            fi
            
            echo ""
            return 0
        else
            error "❌ FAILURE: Cuckoo Filter FPR ${ACTUAL_PCT}% exceeds requirement ≤${BUSINESS_REQUIREMENT_PCT}%"
        fi
        
    else
        warn "Could not extract FPR from test output"
        echo ""
        info "📋 Test output preview:"
        head -20 test-results/cuckoo-validation.txt | sed 's/^/   /'
        echo ""
        error "Failed to validate Cuckoo Filter performance"
    fi
}

show_help() {
    echo "Cuckoo Filter Performance Validator"
    echo ""
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  --help, -h           Show this help message"
    echo "  --requirement RATE   Set business requirement (default: 0.1%)"
    echo "  --verbose, -v        Show verbose output"
    echo ""
    echo "Examples:"
    echo "  $0                          # Validate with default 0.1% requirement"
    echo "  $0 --requirement 0.05       # Validate with 0.05% requirement"
    echo "  $0 --verbose                # Show detailed output"
}

# Parse command line arguments
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --help|-h)
            show_help
            exit 0
            ;;
        --requirement)
            BUSINESS_REQUIREMENT_PCT="$2"
            shift 2
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        *)
            warn "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Main execution
if [ "$VERBOSE" = true ]; then
    info "🔧 Running in verbose mode"
    info "📋 Business requirement: ≤${BUSINESS_REQUIREMENT_PCT}%"
    echo ""
fi

validate_cuckoo_filter_fpr

log "🎉 Cuckoo Filter validation completed!"
