Delete a team and clean up its member agents.

<usage>
- Provide the team name to delete
- Stops and closes all member agents
- Releases mailbox and registry resources
</usage>

<features>
- Graceful shutdown of all team members
- Resource cleanup for mailboxes and registry entries
- Idempotent operation
</features>

<tips>
- Delete teams when their work is complete
- All running tasks within the team are cancelled
- Team name becomes available for reuse after deletion
</tips>
