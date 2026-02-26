module github.com/Gurpartap/agentframe/examples/coding-agent/client

go 1.26

replace github.com/Gurpartap/agentframe => ../../..

replace github.com/Gurpartap/agentframe/examples/coding-agent/server => ../server

require (
	github.com/Gurpartap/agentframe v0.0.0
	github.com/Gurpartap/agentframe/examples/coding-agent/server v0.0.0
)
