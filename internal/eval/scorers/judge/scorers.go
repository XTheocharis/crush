package judge

func NewCodeQualityScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"code_quality",
		client,
		threshold,
		"Code Quality",
		`Evaluate the overall quality of the code. Consider:
- Code organization and structure
- Naming conventions and readability
- Appropriate use of language features
- Absence of code smells
- DRY principle adherence`,
	)
}

func NewCorrectnessScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"correctness",
		client,
		threshold,
		"Correctness",
		`Evaluate whether the code correctly addresses the user's request.
Consider:
- Does the implementation match the stated requirements?
- Are edge cases handled?
- Is the logic sound?
- Are there obvious bugs?`,
	)
}

func NewCompletenessScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"completeness",
		client,
		threshold,
		"Completeness",
		`Evaluate how completely the implementation covers what was requested.
Consider:
- Are all requested features implemented?
- Are there missing error paths?
- Is input validation complete?
- Are all relevant files modified?`,
	)
}

func NewClarityScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"clarity",
		client,
		threshold,
		"Clarity",
		`Evaluate the clarity and readability of the code.
Consider:
- Is the code easy to understand?
- Are variable and function names descriptive?
- Is the control flow clear?
- Are complex operations well-structured?`,
	)
}

func NewSafetyScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"safety",
		client,
		threshold,
		"Safety",
		`Evaluate the safety and security of the code.
Consider:
- Input sanitization and validation
- SQL injection / XSS prevention
- Proper error handling without leaking sensitive data
- Secure use of cryptographic functions
- No hardcoded secrets or credentials`,
	)
}

func NewPerformanceScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"performance",
		client,
		threshold,
		"Performance",
		`Evaluate the performance characteristics of the code.
Consider:
- Algorithmic complexity
- Unnecessary allocations or copies
- Efficient data structure choices
- Proper use of concurrency where beneficial
- N+1 query patterns`,
	)
}

func NewMaintainabilityScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"maintainability",
		client,
		threshold,
		"Maintainability",
		`Evaluate how maintainable the code is for long-term development.
Consider:
- Modularity and separation of concerns
- Ease of extending functionality
- Testability of the code
- Dependency management
- Coupling between components`,
	)
}

func NewErrorHandlingScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"error_handling",
		client,
		threshold,
		"Error Handling",
		`Evaluate the quality of error handling in the code.
Consider:
- Are errors properly propagated?
- Are error messages informative?
- Are edge cases and failure modes covered?
- Is graceful degradation implemented?
- Are resources properly cleaned up on error?`,
	)
}

func NewDocumentationScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"documentation",
		client,
		threshold,
		"Documentation",
		`Evaluate the quality of code documentation.
Consider:
- Are public functions documented?
- Are complex algorithms explained?
- Are package-level docs present?
- Is documentation accurate and up-to-date?
- Are examples provided where helpful?`,
	)
}

func NewConventionsScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"conventions",
		client,
		threshold,
		"Conventions",
		`Evaluate adherence to language and project conventions.
Consider:
- Naming conventions (camelCase, PascalCase, etc.)
- File organization and layout
- Import ordering
- Code formatting consistency
- Use of idiomatic patterns`,
	)
}

func NewTestingQualityScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"testing_quality",
		client,
		threshold,
		"Testing Quality",
		`Evaluate the quality of tests in the code.
Consider:
- Test coverage of key functionality
- Use of table-driven tests
- Proper assertion messages
- Edge case testing
- Test isolation and independence`,
	)
}

func NewEdgeCasesScorer(client LLMClient, threshold float64) *LLMJudgeScorer {
	return newLLMJudgeScorer(
		"edge_cases",
		client,
		threshold,
		"Edge Cases",
		`Evaluate how well the code handles edge cases.
Consider:
- Empty inputs
- Nil or zero values
- Very large inputs
- Concurrent access patterns
- Boundary conditions`,
	)
}
