# HyperCache Grafana Dashboards Guide

This guide covers the comprehensive Grafana dashboards created for monitoring your HyperCache distributed cache system.

## 📊 Dashboard Overview

Your HyperCache monitoring setup includes 5 specialized dashboards:

### 1. **Health Dashboard** (`health-dashboard.json`)
**Focus**: System-wide health monitoring and alerting

**Key Panels:**
- **System Status Overview**: Active nodes count
- **Health Check Success Rate**: Node availability percentage 
- **Error Rate**: System errors and warnings count
- **Request Volume**: Total requests per second
- **Node Status Timeline**: Start/stop events by node
- **Recent Error Log**: Latest warnings and errors

**Best For**: Operations team, incident response, SLA monitoring

---

### 2. **Performance Metrics** (`performance-metrics.json`) 
**Focus**: Latency analysis and performance optimization

**Key Panels:**
- **Request Latency Distribution**: Response time histogram
- **GET Operation Percentiles**: P50, P95, P99 latency for reads
- **PUT Operation Percentiles**: P50, P95, P99 latency for writes  
- **DELETE Operation Percentiles**: P50, P95, P99 latency for deletions
- **Request Rate by Method**: RPS breakdown by operation type
- **HTTP Status Code Distribution**: Success/error response rates
- **Cache Operation Duration Details**: Detailed performance by node

**Best For**: Performance tuning, capacity planning, SLA compliance

---

### 3. **System Components** (`system-components.json`)
**Focus**: Deep dive into HyperCache internal components

**Key Panels:**
- **Cluster Health & Replication**: Inter-node communication status
- **Cuckoo Filter Operations**: Filter efficiency and false positive rates
- **Event Bus Activity**: Message passing between components
- **Node Communication Matrix**: Replication patterns across nodes
- **System Component Status**: Individual component health
- **Replication Events Detail**: Detailed replication log stream
- **Filter Effectiveness**: Hit/miss ratio for Cuckoo filters
- **Node Resource Usage**: Activity breakdown by node and component

**Best For**: System engineers, debugging, architecture analysis

---

### 4. **Operational Dashboard** (`operational-dashboard.json`)
**Focus**: Day-to-day operations and business metrics

**Key Panels:**
- **Cache Hit/Miss Ratio**: Overall cache effectiveness
- **Memory Pressure Indicators**: Eviction and memory alerts
- **Consistency & Replication Health**: Data consistency metrics
- **Data Distribution Across Nodes**: Load balancing effectiveness
- **Network Activity Overview**: Bandwidth usage patterns
- **Persistence Layer Status**: Storage system health
- **Critical Events Timeline**: Important system events
- **System Performance KPIs**: Aggregated performance metrics

**Best For**: DevOps, capacity planning, business metrics

---

### 5. **HyperCache Logs** (`hypercache-logs.json`)
**Focus**: Raw log stream and troubleshooting

**Key Panels:**
- **HyperCache Log Stream**: Real-time structured log viewer

**Best For**: Debugging, troubleshooting, development

## 🚀 Quick Start

### Access Your Dashboards
```bash
# Start the system (if not running)
./scripts/start-system.sh

# Generate test data for better visualizations
./scripts/generate-dashboard-load.sh
```

**Access URLs:**
- Grafana: http://localhost:3000 (admin/admin123)
- All dashboards are auto-provisioned and available immediately

### Dashboard Navigation Tips
1. **Time Range**: Use the time picker (top right) to adjust the monitoring window
2. **Refresh Rate**: Set auto-refresh (top right) for real-time monitoring
3. **Variables**: Some panels support node filtering - look for dropdown menus
4. **Drill-down**: Click on chart elements to filter other panels
5. **Full Screen**: Click panel titles → View → Full screen

## 📈 Understanding the Metrics

### Performance Metrics Explained

**Latency Percentiles:**
- **P50 (Median)**: 50% of requests complete faster than this
- **P95**: 95% of requests complete faster than this (SLA target)
- **P99**: 99% of requests complete faster than this (outlier detection)

**Key Thresholds:**
- **P95 < 10ms**: Excellent performance
- **P95 < 50ms**: Good performance  
- **P95 > 100ms**: Investigate performance issues

### Component Health Indicators

**Cuckoo Filter Metrics:**
- **add_success**: Keys successfully added to filter
- **negative_lookup**: Keys filtered out (avoiding expensive lookups)
- **High negative_lookup ratio**: Good filter effectiveness

**Replication Health:**
- **Successful replications**: Data consistency maintained
- **Failed replications**: Potential consistency issues
- **Node communication matrix**: Even distribution indicates healthy cluster

## 🎯 Monitoring Best Practices

### Daily Operations
1. **Check Health Dashboard**: Start your day with system overview
2. **Review Error Rate**: Investigate any spikes or sustained errors
3. **Monitor Performance**: Ensure P95 latency meets SLAs
4. **Validate Cache Efficiency**: Check hit/miss ratios

### Performance Optimization
1. **Analyze Latency Patterns**: Use performance dashboard to identify bottlenecks
2. **Monitor Node Balance**: Ensure even load distribution
3. **Track Filter Effectiveness**: High negative lookups indicate good performance
4. **Review Network Usage**: Monitor bandwidth for capacity planning

### Troubleshooting Workflow
1. **Health Dashboard**: Identify affected components
2. **System Components**: Dive into specific component issues
3. **Log Dashboard**: Examine detailed error messages
4. **Performance Dashboard**: Correlate with latency impacts

## 🔧 Customization Guide

### Adding Custom Panels
1. **Edit Dashboard**: Click dashboard settings → Edit
2. **Add Panel**: Use "Add panel" button
3. **Query Builder**: Use the Elasticsearch query syntax:
   ```
   component:cache AND action:get_operation
   level:ERROR
   node_id:node-1 AND duration_ms:>100
   ```

### Useful Query Patterns
```bash
# High latency requests
component:http AND action:response AND duration_ms:>50

# Cache operations by node
component:cache AND node_id:node-1

# Replication events
component:cluster AND action:replication

# Filter operations
component:filter AND (action:add_success OR action:negative_lookup)

# Error tracking
level:ERROR OR level:WARN

# Network activity
component:http AND fields.bytes_sent:*
```

### Creating Alerts
1. **Navigate to Alerting** → Alert Rules
2. **Create Rule**: Set query and thresholds
3. **Notification Channels**: Configure Slack, email, etc.
4. **Example Alert**: P95 latency > 100ms for 5 minutes

## 📊 Sample Alert Rules

```yaml
# High Error Rate Alert
- alert: HighErrorRate
  expr: rate(hypercache_errors[5m]) > 0.1
  for: 2m
  annotations:
    summary: "HyperCache error rate is high"

# Node Down Alert  
- alert: NodeDown
  expr: hypercache_nodes_up < 3
  for: 1m
  annotations:
    summary: "HyperCache node is down"

# High Latency Alert
- alert: HighLatency
  expr: hypercache_p95_latency > 100
  for: 5m
  annotations:
    summary: "HyperCache P95 latency is high"
```

## 🔍 Troubleshooting Common Issues

### Dashboard Not Loading
- Check Elasticsearch is running: `curl localhost:9200`
- Verify log indices exist: `curl localhost:9200/_cat/indices`
- Restart Grafana: `docker restart hypercache-grafana`

### No Data in Panels
- Check time range (ensure it covers when HyperCache was running)
- Generate test load: `./scripts/generate-dashboard-load.sh`
- Verify log ingestion: Check raw logs in the log dashboard

### Slow Dashboard Performance
- Reduce time range
- Add more specific filters to queries
- Consider increasing Elasticsearch resources

## 💡 Advanced Features

### Dashboard Variables
Create dashboard variables for dynamic filtering:
1. **Settings** → Variables → Add variable
2. **Query**: `{"find": "terms", "field": "node_id.keyword"}`
3. **Use in panels**: `node_id:$node_id`

### Custom Time Windows
- **Last 15 minutes**: Real-time monitoring
- **Last 1 hour**: Operational troubleshooting  
- **Last 24 hours**: Daily review and trending
- **Last 7 days**: Capacity planning and analysis

---

## 🎉 Next Steps

1. **Generate Load**: Run `./scripts/generate-dashboard-load.sh` for sample data
2. **Customize**: Adjust panels based on your specific monitoring needs
3. **Set Alerts**: Configure notifications for critical metrics
4. **Export**: Backup your customizations using Grafana export

Your comprehensive monitoring setup is now ready! These dashboards provide deep visibility into every aspect of your HyperCache distributed cache system.
