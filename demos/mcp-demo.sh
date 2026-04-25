#!/usr/bin/env sh
set -eu

case "${1:-tools}" in
  tools)
    request='{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
    ;;
  recent)
    request='{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_recent_emails","arguments":{"folder":"INBOX","limit":5}}}'
    ;;
  search)
    request='{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_emails","arguments":{"folder":"INBOX","query":"budget risk"}}}'
    ;;
  *)
    echo "usage: sh demos/mcp-demo.sh [tools|recent|search]" >&2
    exit 2
    ;;
esac

printf '%s\n' "$request" | ./bin/herald-mcp-server --demo
