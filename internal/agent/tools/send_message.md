Send a message to another agent via the mailbox system.

<usage>
- Provide the target agent name and message content
- Messages are delivered asynchronously to the target agent's inbox
- Use this to coordinate work between agents in a team
</usage>

<features>
- Thread-safe message delivery via buffered channels
- Messages include sender, recipient, content, and timestamp
- Non-blocking send with overflow protection
</features>

<tips>
- Check that the target agent exists before sending
- Use meaningful agent names for routing
- Combine with receive operations for request-response patterns
</tips>
