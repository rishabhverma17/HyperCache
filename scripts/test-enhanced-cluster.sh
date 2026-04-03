#!/bin/bash

# Test script for Enhanced HyperCache Cluster Scripts
# This script validates all components work correctly

set -e

PROJECT_ROOT="/Users/rishabhverma/Documents/HobbyProjects/Cache"
cd "$PROJECT_ROOT"

echo "🧪 Enhanced HyperCache Cluster Test Suite"
echo "========================================"
echo ""

# Test 1: Check script files exist
echo "📋 Test 1: Script Files"
echo "----------------------"
scripts_to_check=(
    "scripts/start-cluster-enhanced.sh"
    "scripts/add-node.sh" 
    "scripts/generate-nginx-config.sh"
)

for script in "${scripts_to_check[@]}"; do
    if [[ -f "$script" && -x "$script" ]]; then
        echo "✅ $script - exists and executable"
    else
        echo "❌ $script - missing or not executable"
    fi
done
echo ""

# Test 2: Check template file exists
echo "📋 Test 2: Template Files"
echo "------------------------"
if [[ -f "templates/node-config.yaml.template" ]]; then
    echo "✅ templates/node-config.yaml.template - exists"
    
    # Check template variables
    template_vars=(
        "\${NODE_ID}"
        "\${RESP_PORT}"
        "\${HTTP_PORT}"
        "\${GOSSIP_PORT}"
        "\${CLUSTER_SEEDS}"
        "\${LOG_LEVEL}"
    )
    
    for var in "${template_vars[@]}"; do
        if grep -q "$var" templates/node-config.yaml.template; then
            echo "✅ Template variable $var - found"
        else
            echo "❌ Template variable $var - missing"
        fi
    done
else
    echo "❌ templates/node-config.yaml.template - missing"
fi
echo ""

# Test 3: Check directory structure
echo "📋 Test 3: Directory Structure"
echo "-----------------------------"
directories=(
    "templates"
    "configs"
    "scripts"
)

for dir in "${directories[@]}"; do
    if [[ -d "$dir" ]]; then
        echo "✅ $dir/ - exists"
    else
        echo "❌ $dir/ - missing"
        mkdir -p "$dir"
        echo "  Created $dir/"
    fi
done
echo ""

# Test 4: Syntax check scripts
echo "📋 Test 4: Script Syntax"
echo "------------------------"
for script in "${scripts_to_check[@]}"; do
    if bash -n "$script" 2>/dev/null; then
        echo "✅ $script - syntax OK"
    else
        echo "❌ $script - syntax error"
        bash -n "$script"
    fi
done
echo ""

# Test 5: Template configuration generation test
echo "📋 Test 5: Template Generation Test"
echo "----------------------------------"
test_config="/tmp/test-hypercache-config.yaml"

# Test template substitution
sed -e 's/\${NODE_ID}/test-node/g' \
    -e 's/\${RESP_PORT}/8080/g' \
    -e 's/\${HTTP_PORT}/9080/g' \
    -e 's/\${GOSSIP_PORT}/7080/g' \
    -e 's/\${CLUSTER_SEEDS}/"127.0.0.1:7080"/g' \
    -e 's/\${LOG_LEVEL}/info/g' \
    templates/node-config.yaml.template > "$test_config"

if [[ -f "$test_config" ]]; then
    echo "✅ Template generation - SUCCESS"
    echo "  Generated config size: $(wc -l < "$test_config") lines"
    
    # Validate YAML syntax
    if python3 -c "import yaml; yaml.safe_load(open('$test_config'))" 2>/dev/null; then
        echo "✅ Generated YAML - valid syntax"
    else
        echo "❌ Generated YAML - invalid syntax"
    fi
    
    rm -f "$test_config"
else
    echo "❌ Template generation - FAILED"
fi
echo ""

# Test 6: Port utilities test
echo "📋 Test 6: Port Utilities"
echo "------------------------"
# Test lsof availability
if command -v lsof >/dev/null 2>&1; then
    echo "✅ lsof command - available"
else
    echo "❌ lsof command - not available (needed for port scanning)"
fi

# Test redis-cli availability
if command -v redis-cli >/dev/null 2>&1; then
    echo "✅ redis-cli command - available"
else
    echo "❌ redis-cli command - not available (install with: brew install redis)"
fi

# Test curl availability
if command -v curl >/dev/null 2>&1; then
    echo "✅ curl command - available"
else
    echo "❌ curl command - not available"
fi

# Test nginx availability
if command -v nginx >/dev/null 2>&1; then
    echo "✅ nginx command - available"
else
    echo "❌ nginx command - not available (install with: brew install nginx)"
fi
echo ""

echo "🎉 Test Suite Complete!"
echo ""
echo "📋 Next Steps:"
echo "============="
echo "1. Install missing dependencies if any were reported"
echo "2. Test cluster startup: ./scripts/start-cluster-enhanced.sh --nodes=2"
echo "3. Test node addition: ./scripts/add-node.sh"
echo "4. Generate nginx config: ./scripts/generate-nginx-config.sh"
echo ""
