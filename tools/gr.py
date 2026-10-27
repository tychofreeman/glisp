#!/usr/bin/env python3
"""Call one gorefactor MCP tool and print its result.

    gr.py <module-dir> <tool> '<json args>'
    gr.py . check '{"run_tests": true}'

Starts refactor-mcp (from $REFACTOR_MCP, else refactor-mcp on PATH) for each
call, so undo history does not carry over between calls. Install it with
    go install github.com/tychofreeman/go-refactor-mcp/cmd/refactor-mcp@latest
Exit status: 0 ok, 2 the tool refused or reported an error, 1 protocol error.
"""
import json, os, subprocess, sys

mod, tool, args = sys.argv[1], sys.argv[2], json.loads(sys.argv[3] if len(sys.argv) > 3 else "{}")
p = subprocess.Popen([os.environ.get("REFACTOR_MCP", "refactor-mcp"), "-C", mod], stdin=subprocess.PIPE,
                     stdout=subprocess.PIPE, text=True)

def send(msg):
    p.stdin.write(json.dumps(msg) + "\n"); p.stdin.flush()

def recv(want_id):
    for line in p.stdout:
        msg = json.loads(line)
        if msg.get("id") == want_id:
            return msg

send({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
    "protocolVersion": "2025-06-18", "capabilities": {},
    "clientInfo": {"name": "gr", "version": "0"}}})
recv(1)
send({"jsonrpc": "2.0", "method": "notifications/initialized"})
send({"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": tool, "arguments": args}})
res = recv(2)
p.stdin.close(); p.wait()
if "error" in res:
    print("ERROR:", res["error"]); sys.exit(1)
r = res["result"]
for c in r.get("content", []):
    if c.get("type") == "text":
        print(c["text"])
sys.exit(2 if r.get("isError") else 0)
