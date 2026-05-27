Generate synthetic output for testing and simulation purposes.

<usage>
- Provide content and optional format (text, json, or markdown)
- Returns formatted synthetic output for test assertions
- Useful for validating tool pipelines without side effects
</usage>

<features>
- Supports text, JSON, and markdown output formats
- Deterministic output for reproducible tests
- Zero side effects — pure output generation
</features>

<tips>
- Use in tests to simulate agent responses
- JSON format wraps content in an output object
- Markdown format wraps content in a code block
</tips>
