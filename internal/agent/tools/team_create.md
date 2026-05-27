Create a team of agents for collaborative task execution.

<usage>
- Provide a team name and list of agent names to include
- All specified agents must exist in the registry
- Missing agents are skipped with a warning
</usage>

<features>
- Groups agents by team name for coordinated execution
- Validates agent existence before team creation
- Returns metadata about found and missing agents
</features>

<tips>
- Create agents before adding them to teams
- Use descriptive team names that reflect the task
- Combine with send_message for inter-team coordination
</tips>
