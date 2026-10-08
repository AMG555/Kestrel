// Package workflow implements the graph-based workflow engine.
// Workflows are directed graphs of typed nodes (agent, tool, condition, approval,
// output) compiled to CloudWeGo Eino graphs and executed with run history,
// checkpointing, and SSE progress streaming.
package workflow
