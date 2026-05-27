Stop a running forked sub-agent.

<usage>
- Provide the agent name to stop
- Sends a cancellation signal to the agent's run loop
- Safe to call on agents that are not currently running
</usage>

<features>
- Graceful cancellation via context
- Idempotent operation
- Reports whether the agent was actually running
</features>

<tips>
- Use this to cancel long-running tasks
- Agents clean up their resources on stop
- Stopped agents can be restarted if needed
</tips>
