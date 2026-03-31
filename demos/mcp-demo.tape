Output static/mcp-demo.gif

Set Theme "Catppuccin Mocha"
Set FontSize 13
Set Width 1400
Set Height 700
Set Padding 8

# Show MCP server in action (no interactive app needed)
Type "echo '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}' | ./bin/mcp-server"
Enter
Sleep 2s

Type "echo '{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"list_recent_emails\",\"arguments\":{\"folder\":\"INBOX\",\"limit\":5}}}' | ./bin/mcp-server"
Enter
Sleep 2s

Sleep 1s
